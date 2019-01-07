package gron

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/nwidger/jsoncolor"
)

// Output colors
var (
	StrColor   = color.New(color.FgYellow)
	BraceColor = color.New(color.FgMagenta)
	BareColor  = color.New(color.FgBlue, color.Bold)
	NumColor   = color.New(color.FgRed)
	BoolColor  = color.New(color.FgCyan)
)

// Option bitfields
const (
	OptMonochrome = 1 << iota
	OptNoSort
	OptJSON
)

// Exit codes
const (
	ExitOK = iota
	ExitOpenFile
	ExitReadInput
	ExitFormStatements
	ExitFetchURL
	ExitParseStatements
	ExitJSONEncode
)

// an actionFn represents a main action of the program, it accepts
// an input, output and a bitfield of options; returning an exit
// code and any error that occurred
type ActionFn func(io.Reader, io.Writer, int) (int, error)

// gron is the default action. Given JSON as the input it returns a list
// of assignment statements. Possible options are OptNoSort and OptMonochrome
func Gron(r io.Reader, w io.Writer, opts int) (int, error) {
	var err error

	var conv statementconv
	if opts&OptMonochrome > 0 {
		conv = statementToString
	} else {
		conv = statementToColorString
	}

	ss, err := statementsFromJSON(r, statement{{"json", typBare}})
	if err != nil {
		goto out
	}

	// Go's maps do not have well-defined ordering, but we want a consistent
	// output for a given input, so we must sort the statements
	if opts&OptNoSort == 0 {
		sort.Sort(ss)
	}

	for _, s := range ss {
		if opts&OptJSON > 0 {
			s, err = s.jsonify()
			if err != nil {
				goto out
			}
		}
		fmt.Fprintln(w, conv(s))
	}

out:
	if err != nil {
		return ExitFormStatements, fmt.Errorf("failed to form statements: %s", err)
	}
	return ExitOK, nil
}

// gronStream is like the gron action, but it treats the input as one
// JSON object per line. There's a bit of code duplication from the
// gron action, but it'd be fairly messy to combine the two actions
func GronStream(r io.Reader, w io.Writer, opts int) (int, error) {
	var err error
	errstr := "failed to form statements"
	var i int
	var sc *bufio.Scanner
	var buf []byte

	var conv func(s statement) string
	if opts&OptMonochrome > 0 {
		conv = statementToString
	} else {
		conv = statementToColorString
	}

	// Helper function to make the prefix statements for each line
	makePrefix := func(index int) statement {
		return statement{
			{"json", typBare},
			{"[", typLBrace},
			{fmt.Sprintf("%d", index), typNumericKey},
			{"]", typRBrace},
		}
	}

	// The first line of output needs to establish that the top-level
	// thing is actually an array...
	top := statement{
		{"json", typBare},
		{"=", typEquals},
		{"[]", typEmptyArray},
		{";", typSemi},
	}

	if opts&OptJSON > 0 {
		top, err = top.jsonify()
		if err != nil {
			goto out
		}
	}

	fmt.Fprintln(w, conv(top))

	// Read the input line by line
	sc = bufio.NewScanner(r)
	buf = make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	i = 0
	for sc.Scan() {

		line := bytes.NewBuffer(sc.Bytes())

		var ss statements
		ss, err = statementsFromJSON(line, makePrefix(i))
		i++
		if err != nil {
			goto out
		}

		// Go's maps do not have well-defined ordering, but we want a consistent
		// output for a given input, so we must sort the statements
		if opts&OptNoSort == 0 {
			sort.Sort(ss)
		}

		for _, s := range ss {
			if opts&OptJSON > 0 {
				s, err = s.jsonify()
				if err != nil {
					goto out
				}

			}
			fmt.Fprintln(w, conv(s))
		}
	}
	if err = sc.Err(); err != nil {
		errstr = "error reading multiline input: %s"
	}

out:
	if err != nil {
		return ExitFormStatements, fmt.Errorf(errstr+": %s", err)
	}
	return ExitOK, nil

}

// ungron is the reverse of gron. Given assignment statements as input,
// it returns JSON. The only option is OptMonochrome
func Ungron(r io.Reader, w io.Writer, opts int) (int, error) {
	scanner := bufio.NewScanner(r)
	var maker statementmaker

	if opts&OptJSON > 0 {
		maker = statementFromJSONSpec
	} else {
		maker = statementFromStringMaker
	}

	// Make a list of statements from the input
	var ss statements
	for scanner.Scan() {
		s, err := maker(scanner.Text())
		if err != nil {
			return ExitParseStatements, err
		}
		ss.add(s)
	}
	if err := scanner.Err(); err != nil {
		return ExitReadInput, fmt.Errorf("failed to read input statements")
	}

	// turn the statements into a single merged interface{} type
	merged, err := ss.toInterface()
	if err != nil {
		return ExitParseStatements, err
	}

	// If there's only one top level key and it's "json", make that the top level thing
	mergedMap, ok := merged.(map[string]interface{})
	if ok {
		if len(mergedMap) == 1 {
			if _, exists := mergedMap["json"]; exists {
				merged = mergedMap["json"]
			}
		}
	}

	// Marshal the output into JSON to display to the user
	out := &bytes.Buffer{}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	err = enc.Encode(merged)
	if err != nil {
		return ExitJSONEncode, errors.Wrap(err, "failed to convert statements to JSON")
	}
	j := out.Bytes()

	// If the output isn't monochrome, add color to the JSON
	if opts&OptMonochrome == 0 {
		c, err := colorizeJSON(j)

		// If we failed to colorize the JSON for whatever reason,
		// we'll just fall back to monochrome output, otherwise
		// replace the monochrome JSON with glorious technicolor
		if err == nil {
			j = c
		}
	}

	// For whatever reason, the monochrome version of the JSON
	// has a trailing newline character, but the colorized version
	// does not. Strip the whitespace so that neither has the newline
	// character on the end, and then we'll add a newline in the
	// Fprintf below
	j = bytes.TrimSpace(j)

	fmt.Fprintf(w, "%s\n", j)

	return ExitOK, nil
}

func colorizeJSON(src []byte) ([]byte, error) {
	out := &bytes.Buffer{}
	f := jsoncolor.NewFormatter()

	f.StringColor = StrColor
	f.ObjectColor = BraceColor
	f.ArrayColor = BraceColor
	f.FieldColor = BareColor
	f.NumberColor = NumColor
	f.TrueColor = BoolColor
	f.FalseColor = BoolColor
	f.NullColor = BoolColor

	err := f.Format(out, src)
	if err != nil {
		return out.Bytes(), err
	}
	return out.Bytes(), nil
}

