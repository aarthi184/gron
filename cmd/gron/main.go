package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"gron"

	"github.com/fatih/color"
	"github.com/mattn/go-colorable"
)

// gronVersion stores the current gron version, set at build
// time with the ldflags -X option
var gronVersion = "dev"

func init() {
	flag.Usage = func() {
		h := "Transform JSON (from a file, URL, or stdin) into discrete assignments to make it greppable\n\n"

		h += "Usage:\n"
		h += "  gron [OPTIONS] [FILE|URL|-]\n\n"

		h += "Options:\n"
		h += "  -u, --ungron     Reverse the operation (turn assignments back into JSON)\n"
		h += "  -c, --colorize   Colorize output (default on tty)\n"
		h += "  -m, --monochrome Monochrome (don't colorize output)\n"
		h += "  -s, --stream     Treat each line of input as a separate JSON object\n"
		h += "  -k, --insecure   Disable certificate validation\n"
		h += "  -j, --json       Represent gron data as JSON stream\n"
		h += "      --no-sort    Don't sort output (faster)\n"
		h += "      --version    Print version information\n\n"

		h += "Exit Codes:\n"
		h += fmt.Sprintf("  %d\t%s\n", gron.ExitOK, "OK")
		h += fmt.Sprintf("  %d\t%s\n", gron.ExitOpenFile, "Failed to open file")
		h += fmt.Sprintf("  %d\t%s\n", gron.ExitReadInput, "Failed to read input")
		h += fmt.Sprintf("  %d\t%s\n", gron.ExitFormStatements, "Failed to form statements")
		h += fmt.Sprintf("  %d\t%s\n", gron.ExitFetchURL, "Failed to fetch URL")
		h += fmt.Sprintf("  %d\t%s\n", gron.ExitParseStatements, "Failed to parse statements")
		h += fmt.Sprintf("  %d\t%s\n", gron.ExitJSONEncode, "Failed to encode JSON")
		h += "\n"

		h += "Examples:\n"
		h += "  gron /tmp/apiresponse.json\n"
		h += "  gron http://jsonplaceholder.typicode.com/users/1 \n"
		h += "  curl -s http://jsonplaceholder.typicode.com/users/1 | gron\n"
		h += "  gron http://jsonplaceholder.typicode.com/users/1 | grep company | gron --ungron\n"

		fmt.Fprintf(os.Stderr, h)
	}
}

func main() {
	var (
		ungronFlag     bool
		colorizeFlag   bool
		monochromeFlag bool
		streamFlag     bool
		noSortFlag     bool
		versionFlag    bool
		insecureFlag   bool
		jsonFlag       bool
	)

	flag.BoolVar(&ungronFlag, "ungron", false, "")
	flag.BoolVar(&ungronFlag, "u", false, "")
	flag.BoolVar(&colorizeFlag, "colorize", false, "")
	flag.BoolVar(&colorizeFlag, "c", false, "")
	flag.BoolVar(&monochromeFlag, "monochrome", false, "")
	flag.BoolVar(&monochromeFlag, "m", false, "")
	flag.BoolVar(&streamFlag, "s", false, "")
	flag.BoolVar(&streamFlag, "stream", false, "")
	flag.BoolVar(&noSortFlag, "no-sort", false, "")
	flag.BoolVar(&versionFlag, "version", false, "")
	flag.BoolVar(&insecureFlag, "k", false, "")
	flag.BoolVar(&insecureFlag, "insecure", false, "")
	flag.BoolVar(&jsonFlag, "j", false, "")
	flag.BoolVar(&jsonFlag, "json", false, "")

	flag.Parse()

	// Print version information
	if versionFlag {
		fmt.Printf("gron version %s\n", gronVersion)
		os.Exit(gron.ExitOK)
	}

	// Determine what the program's input should be:
	// file, HTTP URL or stdin
	var rawInput io.Reader
	filename := flag.Arg(0)
	if filename == "" || filename == "-" {
		rawInput = os.Stdin
	} else if gron.ValidURL(filename) {
		r, err := gron.GetURL(filename, insecureFlag, gronVersion)
		if err != nil {
			fatal(gron.ExitFetchURL, err)
		}
		rawInput = r
	} else {
		r, err := os.Open(filename)
		if err != nil {
			fatal(gron.ExitOpenFile, err)
		}
		rawInput = r
	}

	var opts int
	// The monochrome option should be forced if the output isn't a terminal
	// to avoid doing unnecessary work calling the color functions
	switch {
	case colorizeFlag:
		color.NoColor = false
	case monochromeFlag || color.NoColor:
		opts = opts | gron.OptMonochrome
	}
	if noSortFlag {
		opts = opts | gron.OptNoSort
	}
	if jsonFlag {
		opts = opts | gron.OptJSON
	}

	// Pick the appropriate action: gron, ungron or gronStream
	var a gron.ActionFn = gron.Gron
	if ungronFlag {
		a = gron.Ungron
	} else if streamFlag {
		a = gron.GronStream
	}
	exitCode, err := a(rawInput, colorable.NewColorableStdout(), opts)

	if exitCode != gron.ExitOK {
		fatal(exitCode, err)
	}

	os.Exit(gron.ExitOK)
}

func fatal(code int, err error) {
	fmt.Fprintf(os.Stderr, "%s\n", err)
	os.Exit(code)
}
