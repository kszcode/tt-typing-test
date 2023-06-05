package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gdamore/tcell"
	"github.com/mattn/go-isatty"
)

var scr tcell.Screen
var csvMode bool
var jsonMode bool

type result struct {
	Wpm       int       `json:"wpm"`
	Cpm       int       `json:"cpm"`
	Accuracy  float64   `json:"accuracy"`
	Timestamp int64     `json:"timestamp"`
	Mistakes  []mistake `json:"mistakes"`
}

func die(format string, args ...interface{}) {
	if scr != nil {
		scr.Fini()
	}
	fmt.Fprintf(os.Stderr, "ERROR: ")
	fmt.Fprintf(os.Stderr, format, args...)
	fmt.Fprintf(os.Stderr, "\n")
	os.Exit(1)
}

var globalResults []result
var globalInfoAboutTheCurrentTest = ""

func parseConfig(b []byte) map[string]string {
	if b == nil {
		return nil
	}

	cfg := map[string]string{}
	for _, ln := range bytes.Split(b, []byte("\n")) {
		a := strings.SplitN(string(ln), ":", 2)
		if len(a) == 2 {
			cfg[a[0]] = strings.Trim(a[1], " ")
		}
	}

	return cfg
}

func exit(rc int) {
	scr.Fini()

	if jsonMode {
		//Avoid null in serialized JSON.
		for i := range globalResults {
			if globalResults[i].Mistakes == nil {
				globalResults[i].Mistakes = []mistake{}
			}
		}

		b, err := json.Marshal(globalResults)
		if err != nil {
			panic(err)
		}
		os.Stdout.Write(b)
	}

	if csvMode {
		for _, r := range globalResults {
			fmt.Printf("test,%d,%d,%.2f,%d\n", r.Wpm, r.Cpm, r.Accuracy, r.Timestamp)
			for _, m := range r.Mistakes {
				fmt.Printf("mistake,%s,%s\n", m.Word, m.Typed)
			}
		}
	}

	os.Exit(rc)
}

func showReport(
	scr tcell.Screen,
	duration time.Duration,
	correctChars int,
	incorrectChars int,
	attribution string,
	mistakes []mistake,
) {
	cpm := int(float64(correctChars) / (float64(duration) / 60e9))
	wpm := cpm / 5
	accuracy := float64(correctChars) / float64(incorrectChars+correctChars) * 100

	globalResults = append(globalResults, result{wpm, cpm, accuracy, time.Now().Unix(), mistakes})

	mistakeStr := ""
	if attribution != "" {
		attribution = "\n\nAttribution: " + attribution
	}

	if len(mistakes) > 0 {
		mistakeStr = "\nMistakes:    "
		for i, m := range mistakes {
			mistakeStr += m.Word
			if i != len(mistakes)-1 {
				mistakeStr += ", "
			}
		}
	}

	report := fmt.Sprintf("WPM: %9d\nCPM: %9d\nAccuracy:  %.2f%%%s%s%s",
		wpm, cpm, accuracy,
		mistakeStr, attribution, globalInfoAboutTheCurrentTest)

	report = fmt.Sprintf("%s\n", report)
	report = fmt.Sprintf("%s\nTests completed : %d", report, len(globalResults))
	report = fmt.Sprintf("%s\nCharacters      : %d", report, correctChars+incorrectChars)
	report = fmt.Sprintf("%s\nDuration        : %s", report, duration)
	report = fmt.Sprintf("%s\n\nPress ESC to continue.", report)

	scr.Clear()
	drawStringAtCenter(scr, report, tcell.StyleDefault)
	scr.HideCursor()
	scr.Show()

	for {
		if key, ok := scr.PollEvent().(*tcell.EventKey); ok && key.Key() == tcell.KeyEscape {
			return
		} else if ok && key.Key() == tcell.KeyCtrlC {
			exit(1)
		}
	}
}

func createDefaultTyper(scr tcell.Screen) *Typer {
	return NewTyper(scr, true, tcell.ColorDefault,
		tcell.ColorDefault,
		tcell.ColorWhite,
		tcell.ColorGreen,
		tcell.ColorGreen,
		tcell.ColorMaroon)
}

func createTyper(scr tcell.Screen, bold bool, themeName string) *Typer {
	var theme map[string]string

	if b := readResource("themes", themeName); b == nil {
		die("%s does not appear to be a valid theme, try '-list themes' for a list of built in thems.", themeName)
	} else {
		theme = parseConfig(b)
	}

	var bgcol, fgcol, hicol, hicol2, hicol3, errcol tcell.Color
	var err error

	if bgcol, err = makeTcellColorFromHex(theme["bgcol"]); err != nil {
		die("bgcol is not defined and/or a valid hex colour.")
	}
	if fgcol, err = makeTcellColorFromHex(theme["fgcol"]); err != nil {
		die("fgcol is not defined and/or a valid hex colour.")
	}
	if hicol, err = makeTcellColorFromHex(theme["hicol"]); err != nil {
		die("hicol is not defined and/or a valid hex colour.")
	}
	if hicol2, err = makeTcellColorFromHex(theme["hicol2"]); err != nil {
		die("hicol2 is not defined and/or a valid hex colour.")
	}
	if hicol3, err = makeTcellColorFromHex(theme["hicol3"]); err != nil {
		die("hicol3 is not defined and/or a valid hex colour.")
	}
	if errcol, err = makeTcellColorFromHex(theme["errcol"]); err != nil {
		die("errcol is not defined and/or a valid hex colour.")
	}

	return NewTyper(scr, bold, fgcol, bgcol, hicol, hicol2, hicol3, errcol)
}

var usage = `usage: tt [options] [file]

Modes
    -words  WORDFILE    Specifies the file from which words are randomly
                        drawn (default: 1000en).
    -quotes QUOTEFILE   Starts quote mode in which quotes are randomly drawn
                        from the given file. The file should be JSON encoded and
                        have the following form:

                        [{"text": "foo", attribution: "bar"}]

Word Mode
    -n GROUPSZ          Sets the number of words which constitute a group.
    -g NGROUPS          Sets the number of groups which constitute a test.

File Mode
    -start PARAGRAPH    The offset of the starting paragraph, set this to 0 to
                        reset progress on a given file.
Aesthetics
    -showwpm            Display WPM whilst typing.
    -reader-mode        In reader mode, allow to have skip through text using space
    -theme THEMEFILE    The theme to use. 
    -w                  The maximum line length in characters. This option is 
    -notheme            Attempt to use the default terminal theme. 
                        This may produce odd results depending 
                        on the theme colours.
    -blockcursor        Use the default cursor style.
    -bold               Embolden typed text.
                        ignored if -raw is present.
Test Parameters
    -t SECONDS          Terminate the test after the given number of seconds.
    -noskip             Disable word skipping when space is pressed.
    -nobackspace        Disable the backspace key.
    -nohighlight        Disable current and next word highlighting.
    -highlight1         Only highlight the current word.
    -highlight2         Only highlight the next word.

Scripting
    -oneshot            Automatically exit after a single run.
    -noreport           Don't show a report at the end of a test.
    -csv                Print the test results to stdout in the form:
                        [type],[wpm],[cpm],[accuracy],[timestamp].
    -json               Print the test output in JSON.
    -raw                Don't reflow STDIN text or show one paragraph at a time.
                        Note that line breaks are determined exclusively by the
                        input.
    -multi              Treat each input paragraph as a self contained test.

Misc
    -list TYPE          Lists internal resources of the given type.
                        TYPE=[themes|quotes|words]

Version
    -v                  Print the current version.
`

func saveMistakes(mistakes []mistake) {
	var db []mistake

	if err := readValue(MISTAKE_DB, &db); err != nil {
		db = nil
	}

	db = append(db, mistakes...)
	writeValue(MISTAKE_DB, db)
}

// main execution point
func main() {

	// Word configuration variables
	var wordCount int
	var groupCount int

	// Highlight configuration variables
	var disableHighlightCurrent bool
	var disableHighlightNext bool
	var disableHighlight bool

	// Miscellaneous configuration variables
	var noSkip bool
	var readerMode bool
	var disableBackspace bool
	var disableReport bool
	var disableTheme bool
	var useNormalCursor bool
	var maxLineLength int
	var timeoutDuration int
	var startParagraphIndex int

	// File and mode configuration variables
	var rawMode bool
	var oneShotMode bool
	var listFlag string
	var wordFilePath string
	var quoteFilePath string
	var themeName string
	var showWordsPerMinute bool
	var multiMode bool
	var versionFlag bool
	var boldFlag bool

	// Extract type test function variable
	var customFunctionToExtractNextListOfSegments func() []segment

	// Set the command line flags
	flag.IntVar(&wordCount, "n", 50, "")
	flag.IntVar(&groupCount, "g", 1, "")
	flag.IntVar(&startParagraphIndex, "start", -1, "")
	flag.IntVar(&maxLineLength, "w", 108*5, "") // The default screen size is 540 which is very wide.
	flag.IntVar(&timeoutDuration, "t", -1, "")
	flag.BoolVar(&versionFlag, "v", false, "")
	flag.StringVar(&wordFilePath, "words", "", "")
	flag.StringVar(&quoteFilePath, "quotes", "", "")
	flag.BoolVar(&showWordsPerMinute, "showwpm", false, "")
	flag.BoolVar(&noSkip, "noskip", false, "")
	flag.BoolVar(&readerMode, "reader-mode", true,
		"In reader mode, allow to have skip through text using space")
	flag.BoolVar(&useNormalCursor, "blockcursor", false, "")
	flag.BoolVar(&disableBackspace, "nobackspace", false, "")
	flag.BoolVar(&disableTheme, "notheme", false, "")
	flag.BoolVar(&oneShotMode, "oneshot", false, "")
	flag.BoolVar(&disableHighlight, "nohighlight", false, "")
	flag.BoolVar(&disableHighlightCurrent, "highlight2", false, "")
	flag.BoolVar(&disableHighlightNext, "highlight1", false, "")
	flag.BoolVar(&disableReport, "noreport", false, "")
	flag.BoolVar(&boldFlag, "bold", false, "")
	flag.BoolVar(&csvMode, "csv", false, "")
	flag.BoolVar(&jsonMode, "json", false, "")
	flag.BoolVar(&rawMode, "raw", false, "")
	flag.BoolVar(&multiMode, "multi", false, "")
	flag.StringVar(&themeName, "theme", "default", "")
	flag.StringVar(&listFlag, "list", "", "")

	// Assign a custom function to handle usage
	flag.Usage = func() { os.Stdout.Write([]byte(usage)) }
	flag.Parse()

	// List the files in the specified directory
	if listFlag != "" {
		prefix := listFlag + "/"
		for filePath, _ := range packedFiles {
			if strings.Index(filePath, prefix) == 0 {
				_, fileName := filepath.Split(filePath)
				fmt.Println(fileName)
			}
		}

		os.Exit(0)
	}

	// Print version information
	if versionFlag {
		fmt.Fprintf(os.Stderr, "tt version 0.4.2\n")
		os.Exit(1)
	}

	// Disable theme if requested
	if disableTheme {
		os.Setenv("TCELL_TRUECOLOR", "disable")
	}

	// Function to reflow the text to fit the screen
	reflowTextForScreen := func(inputText string) string {
		screenWidth, _ := scr.Size()
		// Adjust window size based on screen size
		windowSize := maxLineLength
		if windowSize > screenWidth {
			windowSize = screenWidth - 8
		}

		// Replace multiple spaces with single space
		inputText = regexp.MustCompile("\\s+").ReplaceAllString(inputText, " ")
		return strings.Replace(
			wordWrap(strings.Trim(inputText, " "), windowSize),
			"\n", " \n", -1)
	}

	// Assign the test generation function based on input configuration
	switch {
	case wordFilePath != "":
		customFunctionToExtractNextListOfSegments = generateWordTest(wordFilePath, wordCount, groupCount)
	case quoteFilePath != "":
		customFunctionToExtractNextListOfSegments = generateQuoteTest(quoteFilePath)
	case !isatty.IsTerminal(os.Stdin.Fd()):
		buffer, err := io.ReadAll(os.Stdin)
		if err != nil {
			panic(err)
		}
		customFunctionToExtractNextListOfSegments = generateTestFromData(buffer, rawMode, multiMode)
	case len(flag.Args()) > 0:
		typingTextPath := flag.Args()[0]
		customFunctionToExtractNextListOfSegments = generateTestFromFile(typingTextPath, startParagraphIndex)
	default:
		customFunctionToExtractNextListOfSegments = generateWordTest("1000en", wordCount, groupCount)
	}

	var err error
	scr, err = tcell.NewScreen()
	if err != nil {
		panic(err)
	}

	// Initialize the screen
	if err := scr.Init(); err != nil {
		panic(err)
	}

	// Defer function to finalize the screen in case of error
	defer func() {
		if recovery := recover(); recovery != nil {
			scr.Fini()
			panic(recovery)
		}
	}()

	// Initialize the typer
	var typingMachine *Typer
	if disableTheme {
		typingMachine = createDefaultTyper(scr)
	} else {
		typingMachine = createTyper(scr, boldFlag, themeName)
	}

	// Update highlighting styles based on flags
	if disableHighlightNext || disableHighlight {
		typingMachine.currentWordStyle = typingMachine.nextWordStyle
		typingMachine.nextWordStyle = typingMachine.defaultStyle
	}
	if disableHighlightCurrent || disableHighlight {
		typingMachine.currentWordStyle = typingMachine.defaultStyle
	}

	// Update typer options
	typingMachine.SkipWord = !noSkip
	typingMachine.ReaderMode = readerMode
	typingMachine.DisableBackspace = disableBackspace
	typingMachine.BlockCursor = useNormalCursor
	typingMachine.ShowWpm = showWordsPerMinute

	// Adjust timeout duration if specified
	if timeoutDuration != -1 {
		timeoutDuration *= 1e9
	}

	// Initialize segment list and index
	var lstx2OfSegmentsFound [][]segment
	var idxOfPreparedSegments = 0

	// Typing loop
	for {
		// Generate segments
		if idxOfPreparedSegments >= len(lstx2OfSegmentsFound) {
			lstx2OfSegmentsFound = append(lstx2OfSegmentsFound, customFunctionToExtractNextListOfSegments())
			// Note: customFunctionToExtractNextListOfSegments should be a different abstraction
			// it should be called on an object like structure,
			// where if we want we can manipulate the object before we call this function
		}

		// Handle no segment found
		listOfSegmentsToType := lstx2OfSegmentsFound[idxOfPreparedSegments]
		if listOfSegmentsToType == nil {
			fmt.Printf("No text found on index %d\n", idxOfPreparedSegments)
			exit(0)
		}

		// Reflow text for screen if not in raw mode
		if !rawMode {
			for i, _ := range listOfSegmentsToType {
				listOfSegmentsToType[i].Text = reflowTextForScreen(listOfSegmentsToType[i].Text)
			}
		}

		// Start typing
		errorCount, correctCount, duration, returnCode, mistakes :=
			typingMachine.Start(listOfSegmentsToType, time.Duration(timeoutDuration))
		saveMistakes(mistakes)

		// Handle typing return code
		switch returnCode {
		case UserAskedForNext:
			idxOfPreparedSegments++
		case UserAskedForPrevious:
			if idxOfPreparedSegments == 0 {
				// TODO: on customFunctionToExtractNextListOfSegments
				// needs to be recalculated because now we need to shift the start index back if it permits it
			} else if idxOfPreparedSegments > 0 {
				idxOfPreparedSegments--
			}
		case UserCompleted:
			if !disableReport {
				attribution := ""
				if len(listOfSegmentsToType) == 1 {
					attribution = listOfSegmentsToType[0].Attribution
				}

				showReport(scr, duration, correctCount, errorCount, attribution, mistakes)
			}
			if oneShotMode {
				exit(0)
			}

			idxOfPreparedSegments++
		case UserAskedForSigInt:
			exit(1)

		case TyperAppResize:
			// Resize events restart the test, this shouldn't be a problem in the vast majority of cases
			// and allows us to avoid baking rewrapping logic into the typer.

			// TODO: implement state-preserving resize (maybe)
		}
	}
}
