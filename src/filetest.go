package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// generateTestFromFile is a function that accepts a file path and a starting paragraph number.
// It reads the file and segments it into paragraphs.
// The function returns a closure that, when invoked, returns the next paragraph as a segment.
func generateTestFromFile(filePath string, startParagraph int) func() []segment {
	var listOfParagraphs []string  // Contains file contents segmented into paragraphs
	var fileStateDB map[string]int // Map to keep track of the last read paragraph for each file
	var err error                  // error variable to catch errors

	// Convert the given path to its absolute path
	if filePath, err = filepath.Abs(filePath); err != nil {
		panic(err) // Terminate the program if the path conversion fails
	}

	// Attempt to load the current file state from disk
	if err := readValue(FILE_STATE_DB, &fileStateDB); err != nil {
		// If an error occurs, create a new empty map
		fileStateDB = map[string]int{}
	}

	// If a specific starting paragraph is provided, update the file state
	if startParagraph != -1 {
		fileStateDB[filePath] = startParagraph
		writeValue(FILE_STATE_DB, fileStateDB) // Persist the new state to disk
	}

	// Get the current paragraph index from the state
	currentParagraphIdx := fileStateDB[filePath] - 1

	// Read the file content
	if fileContentBytes, err := os.ReadFile(filePath); err != nil {
		die("Failed to read %s.", filePath) // Exit the program if the file reading fails
	} else {
		// Convert the file content to string and split it into paragraphs
		listOfParagraphs = getParagraphs(string(fileContentBytes))
	}

	// Return a closure function
	return func() []segment {
		// Increment the current paragraph index
		currentParagraphIdx++
		// step back the save, because this way we can resume the test from the previous paragraph if needed
		// requires restart to step back, so it should be safe enough for now, otherwise we need to
		// change the logic for customFunctionToExtractNextListOfSegments
		stepBackSize := 1
		// Update the state with the new paragraph index
		fileStateDB[filePath] = currentParagraphIdx - stepBackSize
		// Persist the updated state to disk
		writeValue(FILE_STATE_DB, fileStateDB)

		// If the current paragraph index exceeds the number of paragraphs, return nil
		if currentParagraphIdx >= len(listOfParagraphs) {
			return nil
		}
		// get the last 81 characters of the filePath
		filePathShort := filePath
		maxSizeOfFilePath := 54
		if len(filePathShort) > maxSizeOfFilePath {
			filePathShort = fmt.Sprintf("..%s", filePath[len(filePath)-maxSizeOfFilePath:])
		}
		globalInfoAboutTheCurrentTest = fmt.Sprintf("\nParagraph: %d/%d\n\nFile: %s",
			currentParagraphIdx+1, len(listOfParagraphs), filePathShort)

		// Return the current paragraph as a segment
		return []segment{segment{listOfParagraphs[currentParagraphIdx], "", currentParagraphIdx}}
	}
}
