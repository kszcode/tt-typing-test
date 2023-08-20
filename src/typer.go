package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/gdamore/tcell"
)

const (
	UserCompleted = iota
	UserAskedForSigInt
	UserTypedEscape
	UserAskedForPrevious
	UserAskedForNext
	TyperAppResize

	yLineMultiplier = 2 // so it leaves space for the typed text, which will show the errors as well
)

type segment struct {
	Text           string `json:"text"`
	Attribution    string `json:"attribution"`
	ParagraphIndex int    `json:"paragraph_index"`
}

type mistake struct {
	Word  string `json:"word"`
	Typed string `json:"typed"`
}

type TyperScreen struct {
	Screen           tcell.Screen
	SkipWord         bool
	ReaderMode       bool
	ShowWpm          bool
	DisableBackspace bool
	BlockCursor      bool
	Tty              io.Writer

	currentWordStyle    tcell.Style
	nextWordStyle       tcell.Style
	incorrectSpaceStyle tcell.Style
	incorrectStyle      tcell.Style
	correctStyle        tcell.Style
	defaultStyle        tcell.Style
}

func NewTyper(
	screen tcell.Screen,
	emboldenTypedText bool,
	fgColor, bgColor, hiColor, hiColor2, hiColor3, errColor tcell.Color,
) *TyperScreen {
	var tty io.Writer
	def := tcell.StyleDefault.
		Foreground(fgColor).
		Background(bgColor)

	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	// Will fail on windows, but tty is still mostly usable via tcell
	if err != nil {
		tty = io.Discard
	}

	correctStyle := def.Foreground(hiColor)
	if emboldenTypedText {
		correctStyle = correctStyle.Bold(true)
	}

	return &TyperScreen{
		Screen:   screen,
		SkipWord: true,
		Tty:      tty,

		defaultStyle:        def,
		correctStyle:        correctStyle,
		currentWordStyle:    def.Foreground(hiColor2),
		nextWordStyle:       def.Foreground(hiColor3),
		incorrectStyle:      def.Foreground(errColor),
		incorrectSpaceStyle: def.Background(errColor),
	}
}

func (t *TyperScreen) Start(
	listOfSegmentsToType []segment,
	timeout time.Duration,
) (
	numErrors,
	numCorrect int,
	duration time.Duration,
	returnCode int,
	mistakes []mistake,
) {
	timeLeft := timeout

	for i, segmentToType := range listOfSegmentsToType {
		startImmediately := true
		var testDuration time.Duration
		var errCount, correctCount int
		var mistakesMadeDuringTest []mistake

		if i == 0 {
			startImmediately = false
		}

		errCount, correctCount, returnCode, testDuration, mistakesMadeDuringTest =
			t.start(segmentToType.Text, timeLeft, startImmediately, segmentToType.Attribution)

		numErrors += errCount
		numCorrect += correctCount
		duration += testDuration
		mistakes = append(mistakes, mistakesMadeDuringTest...)

		if timeout != -1 {
			timeLeft -= testDuration
			if timeLeft <= 0 {
				return
			}
		}

		if returnCode != UserCompleted {
			return
		}
	}

	return
}

func extractMistypedWords(
	text []rune,
	typed []rune,
	readMode bool,
) (mistakes []mistake) {
	var word []rune
	var typedWord []rune
	isMismatched := false

	for i := range text {
		if text[i] == ' ' {
			strTypedWord := string(typedWord)
			lengthOfTypedWord := len(strTypedWord)
			if isMismatched && (lengthOfTypedWord > 0) {
				mistakes = append(mistakes, mistake{string(word), strTypedWord})
			}

			word = word[:0]
			typedWord = typedWord[:0]
			isMismatched = false
			continue
		}

		if text[i] != typed[i] {
			isMismatched = true
		}

		if text[i] == 0 {
			word = append(word, '_')
		} else {
			word = append(word, text[i])
		}

		if typed[i] == 0 {
			if !readMode {
				typedWord = append(typedWord, '_')
			}
		} else {
			typedWord = append(typedWord, typed[i])
		}
	}

	if isMismatched {
		mistakes = append(mistakes, mistake{string(word), string(typedWord)})
	}

	return
}

func (t *TyperScreen) start(
	textToType string,
	timeLimit time.Duration,
	startImmediately bool,
	attribution string,
) (
	numErrors int,
	numCorrect int,
	returnCode int,
	duration time.Duration,
	mistakes []mistake,
) {

	var startTime time.Time
	referenceText := []rune(textToType)
	userTypedText := make([]rune, len(referenceText))

	screenWidth, screenHeight := t.Screen.Size()
	numCols, numRows := calcStringDimensions(textToType)
	xStartLeftSideOfScreen := (screenWidth - numCols) / 2

	yStartTopSideOfSideOfScreen := (screenHeight - numRows*yLineMultiplier) / 2
	if yStartTopSideOfSideOfScreen < 0 {
		yStartTopSideOfSideOfScreen = 0
	}

	if !t.BlockCursor {
		t.Tty.Write([]byte("\033[5 q"))

		// Assumes the original cursor shape was a block (the one true cursor shape),
		// there doesn't appear to be a good way to save/restore the shape if the user has changed it from the otcs.
		defer t.Tty.Write([]byte("\033[2 q"))
	}

	t.Screen.SetStyle(t.defaultStyle)

	// cursorPositionInText represents the current position of the typist within the text to be typed.
	// It tracks the position where the next character is to be typed or erased.
	// This variable starts at 0 and increases as characters are typed, and decreases when characters are erased.
	cursorPositionInText := 0

	tickerCloser := make(chan bool)

	// Inject nil events into the main event loop at regular intervals to force an update
	ticker := func() {
		for {
			select {
			case <-tickerCloser:
				return
			default:
			}

			halfSecond := time.Duration(5e8)
			time.Sleep(halfSecond)
			t.Screen.PostEventWait(nil)
		}
	}

	go ticker()
	defer close(tickerCloser)

	if startImmediately {
		startTime = time.Now()
	}

	t.Screen.Clear()
	for {

		t.redraw(referenceText, userTypedText, cursorPositionInText,
			xStartLeftSideOfScreen, yStartTopSideOfSideOfScreen,
			numCols, numRows, attribution, startTime, timeLimit)

		ev := t.Screen.PollEvent()

		switch ev := ev.(type) {
		case *tcell.EventResize:
			returnCode = TyperAppResize
			return
		case *tcell.EventKey:
			if runtime.GOOS != "windows" && ev.Key() == tcell.KeyBackspace { // Control+backspace on unix terms
				if !t.DisableBackspace {
					t.deleteWord(&cursorPositionInText, referenceText, userTypedText)
				}
				continue
			}

			if startTime.IsZero() {
				startTime = time.Now()
			}

			switch key := ev.Key(); key {
			case tcell.KeyCtrlC:
				returnCode = UserAskedForSigInt

				return
			case tcell.KeyEscape:
				returnCode = UserTypedEscape

				return
			case tcell.KeyCtrlL:
				t.Screen.Sync()

			case tcell.KeyRight:
				returnCode = UserAskedForNext
				return

			case tcell.KeyLeft:
				returnCode = UserAskedForPrevious
				return

			case tcell.KeyCtrlW:
				if !t.DisableBackspace {
					t.deleteWord(&cursorPositionInText, referenceText, userTypedText)
				}

			case tcell.KeyBackspace, tcell.KeyBackspace2:
				if !t.DisableBackspace {
					if ev.Modifiers() == tcell.ModAlt || ev.Modifiers() == tcell.ModCtrl {
						t.deleteWord(&cursorPositionInText, referenceText, userTypedText)
					} else {
						if cursorPositionInText == 0 {
							break
						}

						cursorPositionInText--

						for cursorPositionInText > 0 && referenceText[cursorPositionInText] == '\n' {
							cursorPositionInText--
						}
					}
				}
			case tcell.KeyEnter:
				if cursorPositionInText < len(referenceText) {
					if !t.ReaderMode && cursorPositionInText > 0 {
						prevCharacterIsSpace := referenceText[cursorPositionInText-1] == ' '
						if prevCharacterIsSpace && referenceText[cursorPositionInText] != ' ' { // Do nothing on word boundaries.
							break
						}
					}

					for cursorPositionInText < len(referenceText) && referenceText[cursorPositionInText] != ' ' && referenceText[cursorPositionInText] != '\n' {
						userTypedText[cursorPositionInText] = 0
						cursorPositionInText++
					}

					if cursorPositionInText < len(referenceText) {
						userTypedText[cursorPositionInText] = referenceText[cursorPositionInText]
						cursorPositionInText++
					}
				}

			case tcell.KeyRune:
				if cursorPositionInText < len(userTypedText) {
					// feed the character into the userTypedText buffer
					userTypedText[cursorPositionInText] = ev.Rune()
					cursorPositionInText++

					for cursorPositionInText < len(referenceText) && referenceText[cursorPositionInText] == '\n' {
						userTypedText[cursorPositionInText] = referenceText[cursorPositionInText]
						cursorPositionInText++
					}
				}

				if cursorPositionInText == len(referenceText) {
					numErrors, numCorrect, mistakes, duration = t.calculateStatistics(startTime, referenceText, userTypedText, cursorPositionInText)
					returnCode = UserCompleted
					return
				}
			}
		default: // tick
			if timeLimit != -1 && !startTime.IsZero() && timeLimit <= time.Now().Sub(startTime) {
				numErrors, numCorrect, mistakes, duration = t.calculateStatistics(startTime, referenceText, userTypedText, cursorPositionInText)
				returnCode = UserCompleted
				return
			}

			t.redraw(referenceText, userTypedText, cursorPositionInText,
				xStartLeftSideOfScreen, yStartTopSideOfSideOfScreen,
				numCols, numRows, attribution, startTime, timeLimit)
		}
	}
}

func (t *TyperScreen) deleteWord(cursorPositionInText *int, referenceText []rune, userTypedText []rune) {
	if *cursorPositionInText == 0 {
		return
	}

	*cursorPositionInText--

	for *cursorPositionInText > 0 && (referenceText[*cursorPositionInText] == ' ' || referenceText[*cursorPositionInText] == '\n') {
		*cursorPositionInText--
	}

	for *cursorPositionInText > 0 && referenceText[*cursorPositionInText] != ' ' && referenceText[*cursorPositionInText] != '\n' {
		*cursorPositionInText--
	}

	if referenceText[*cursorPositionInText] == ' ' || referenceText[*cursorPositionInText] == '\n' {
		userTypedText[*cursorPositionInText] = referenceText[*cursorPositionInText]
		*cursorPositionInText++
	}
}

func (t *TyperScreen) redraw(
	referenceText []rune,
	userTypedText []rune,
	cursorPositionInText int,
	xStartLeftSideOfScreen int,
	yStartTopSideOfSideOfScreen int,
	numCols int,
	numRows int,
	attribution string,
	startTime time.Time,
	timeLimit time.Duration,
) {
	cursorX := xStartLeftSideOfScreen
	cursorY := yStartTopSideOfSideOfScreen
	inWord := -1

	for i := range referenceText {
		style := t.defaultStyle

		characterInSegment := referenceText[i]
		if characterInSegment == '\n' {
			cursorY += yLineMultiplier
			cursorX = xStartLeftSideOfScreen
			if inWord != -1 {
				inWord++
			}
			continue
		}

		if i == cursorPositionInText {
			t.Screen.ShowCursor(cursorX, cursorY)
			inWord = 0
		}

		if i >= cursorPositionInText {
			if characterInSegment == ' ' {
				inWord++
			} else if inWord == 0 {
				style = t.currentWordStyle
			} else if inWord == 1 {
				style = t.nextWordStyle
			} else {
				style = t.defaultStyle
			}
		} else if characterInSegment != userTypedText[i] {
			if characterInSegment == ' ' {
				style = t.incorrectSpaceStyle
			} else {
				style = t.incorrectStyle
			}
		} else {
			style = t.correctStyle
		}

		t.Screen.SetContent(cursorX, cursorY, characterInSegment, nil, style)
		// only type the character in the row below if it is different from the correct character
		if referenceText[i] != userTypedText[i] {
			t.Screen.SetContent(cursorX, cursorY+1, userTypedText[i], nil, style)
		}

		cursorX++
	}

	attributionWidth, attributionHeight := calcStringDimensions(attribution)
	drawString(
		t.Screen,
		xStartLeftSideOfScreen+numCols-attributionWidth,
		yStartTopSideOfSideOfScreen+numRows*yLineMultiplier+1,
		attribution,
		-1,
		t.defaultStyle,
	)

	if timeLimit != -1 && !startTime.IsZero() {
		remaining := timeLimit - time.Now().Sub(startTime)
		drawString(t.Screen,
			xStartLeftSideOfScreen+numCols/2,
			yStartTopSideOfSideOfScreen+numRows*yLineMultiplier+attributionHeight+1,
			"      ",
			-1,
			t.defaultStyle,
		)
		drawString(t.Screen,
			xStartLeftSideOfScreen+numCols/2,
			yStartTopSideOfSideOfScreen+numRows*yLineMultiplier+attributionHeight+1,
			strconv.Itoa(int(remaining/1e9)+1),
			-1,
			t.defaultStyle,
		)
	}

	if t.ShowWpm && !startTime.IsZero() {
		//calculateStatistics()
		_, numCorrect, _, duration := t.calculateStatistics(startTime, referenceText, userTypedText, cursorPositionInText)
		//returnCode = UserCompleted
		if duration > 1e7 { // Avoid flashing large numbers on test start.
			wpm := int((float64(numCorrect) / 5) / (float64(duration) / 60e9))
			drawString(t.Screen,
				xStartLeftSideOfScreen+numCols/2-4,
				yStartTopSideOfSideOfScreen-2,
				fmt.Sprintf("WPM: %-10d\n", wpm),
				-1,
				t.defaultStyle,
			)
		}
	}

	t.Screen.Show()
}

func (t *TyperScreen) calculateStatistics(
	startTime time.Time,
	referenceText []rune, userTypedText []rune, cursorPositionInText int,
) (
	numErrors, numCorrect int, mistakes []mistake, duration time.Duration,
) {

	numErrors = 0
	numCorrect = 0

	mistakes = extractMistypedWords(
		referenceText[:cursorPositionInText], userTypedText[:cursorPositionInText], t.ReaderMode)

	for i := 0; i < cursorPositionInText; i++ {
		if referenceText[i] != '\n' {
			if referenceText[i] != userTypedText[i] {
				numErrors++
			} else {
				numCorrect++
			}
		}
	}

	duration = time.Now().Sub(startTime)
	return
}
