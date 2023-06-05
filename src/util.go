package main

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gdamore/tcell"
)

var CONFIG_DIRS []string

func init() {
	home, _ := os.LookupEnv("HOME")

	CONFIG_DIRS = []string{
		filepath.Join(home, ".tt"),
		"/etc/tt",
	}
}

type cell struct {
	c     rune
	style tcell.Style
}

func dbgPrintf(scr tcell.Screen, format string, args ...interface{}) {
	for i := 0; i < 80; i++ {
		for j := 0; j < 80; j++ {
			scr.SetContent(i, j, ' ', nil, tcell.StyleDefault)
		}
	}
	drawString(scr, 0, 0, fmt.Sprintf(format, args...), -1, tcell.StyleDefault)
}

func getParagraphs(s string) []string {
	//s = strings.ReplaceAll(s, ".", ".\n\n") // split also per sentence
	s = strings.Replace(s, "\r", "", -1)
	s = regexp.MustCompile("\n\n+").ReplaceAllString(s, "\n\n")
	return strings.Split(strings.Trim(s, "\n"), "\n\n")
}

func wordWrapBytes(s []byte, n int) {
	sp := 0
	sz := 0

	for i := 0; i < len(s); i++ {
		sz++

		if s[i] == '\n' {
			s[i] = ' '
		}

		if s[i] == ' ' {
			sp = i
		}

		if sz > n {
			if sp != 0 {
				s[sp] = '\n'
			}

			sz = i - sp
		}
	}

}

func wordWrap(s string, n int) string {
	r := []byte(s)
	wordWrapBytes(r, n)
	return string(r)
}

func init() {
	rand.Seed(time.Now().Unix())
}

func randomText(n int, words []string) string {
	r := ""

	var last string
	for i := 0; i < n; i++ {
		w := words[rand.Int()%len(words)]
		for last == w {
			w = words[rand.Int()%len(words)]
		}

		r += w
		if i != n-1 {
			r += " "
		}

		last = w
	}

	return strings.Replace(r, "\n", " \n", -1)
}

func stringToCells(s string) []cell {
	a := make([]cell, len(s))
	s = strings.TrimRight(s, "\n ")

	len := 0
	for _, r := range s {
		a[len].c = r
		a[len].style = tcell.StyleDefault
		len++
	}

	return a[:len]
}

func drawString(scr tcell.Screen, x, y int, s string, cursorIdx int, style tcell.Style) {
	startX := x

	for i, c := range s {
		if c == '\n' {
			y++
			x = startX
		} else {
			scr.SetContent(x, y, c, nil, style)
			if i == cursorIdx {
				scr.ShowCursor(x, y)
			}

			x++
		}
	}

	if cursorIdx == len(s) {
		scr.ShowCursor(x, y)
	}
}

func drawStringAtCenter(scr tcell.Screen, s string, style tcell.Style) {
	nc, nr := calcStringDimensions(s)
	sw, sh := scr.Size()

	x := (sw - nc) / 2
	y := (sh - nr) / 2

	drawString(scr, x, y, s, -1, style)
}

// calcStringDimensions calculates the dimensions of a string, returning the number
// of columns (maximum line length) and the number of rows (number of lines).
func calcStringDimensions(inputStr string) (numColumns, numRows int) {
	// Case when string is empty.
	if inputStr == "" {
		return 0, 0
	}

	// Initialize character count in a line.
	charCountInLine := 0

	// Iterate over each character in the string.
	for _, char := range inputStr {
		// Check if character is a new line.
		if char == '\n' {
			// If it is, increment the number of rows.
			numRows++
			// If the character count in this line is greater than the maximum columns seen so far,
			// update the number of columns.
			if charCountInLine > numColumns {
				numColumns = charCountInLine
			}
			// Reset the character count for the new line.
			charCountInLine = 0
		} else {
			// If it's not a new line, increment the character count for this line.
			charCountInLine++
		}
	}

	// Increment row count to account for the last line (or single line if no '\n' characters)
	numRows++
	// Check if the character count in the last line is greater than the maximum columns seen so far.
	if charCountInLine > numColumns {
		numColumns = charCountInLine
	}

	// Return the number of columns and rows.
	return
}

// makeTcellColorFromHex converts a hex color string to a tcell.Color value.
func makeTcellColorFromHex(hexColor string) (tcell.Color, error) {
	// Validate that the string is a 7-character hex color (like "#FFFFFF").
	if len(hexColor) != 7 || hexColor[0] != '#' {
		return 0, fmt.Errorf("%s is not a valid hex color", hexColor)
	}

	// hexCharToDecimal function converts a hex character into a decimal number.
	hexCharToDecimal := func(hexChar byte) int32 {
		// If the character is greater than '9', it's a letter.
		if hexChar > '9' {
			// Handle lowercase letters
			if hexChar >= 'a' {
				return (int32)(hexChar - 'a' + 10)
			} else { // Handle uppercase letters
				return (int32)(hexChar - 'A' + 10)
			}
		} else { // If it's less than or equal to '9', it's a digit.
			return (int32)(hexChar - '0')
		}
	}

	// Extract and convert each RGB component from the hex color.
	redComponent := hexCharToDecimal(hexColor[1])<<4 | hexCharToDecimal(hexColor[2])
	greenComponent := hexCharToDecimal(hexColor[3])<<4 | hexCharToDecimal(hexColor[4])
	blueComponent := hexCharToDecimal(hexColor[5])<<4 | hexCharToDecimal(hexColor[6])

	// Return a new tcell.Color object created from the RGB components.
	return tcell.NewRGBColor(redComponent, greenComponent, blueComponent), nil
}

func readResource(typ, name string) []byte {
	if name == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			panic(err)
		}

		return b
	}

	if b, err := os.ReadFile(name); err == nil {
		return b
	}

	for _, d := range CONFIG_DIRS {
		if b, err := os.ReadFile(filepath.Join(d, typ, name)); err == nil {
			return b
		}
	}

	return readPackedFile(filepath.Join(typ, name))
}
