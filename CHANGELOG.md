# 0.5.1
Wish list:
- When quiting the application show the accuracy report, and statistics
- When there are no more paragraphs to type, show the accuracy report, 
  and statistics and a message saying that the test is over.
- Test only some fingers, meaning that it will autotype the rest of the 
  fingers. This will be useful for practicing individual fingers.

# 0.5.0:
- Replaced `ioutil.ReadAll` with `io.ReadAll` in `main` function in `tt.go`.
- Renamed `testFn` to `extractTypeTestFunction` in `tt.go`.
- Changed default value for `maxLineLen` flag in `tt.go`.
- Renamed `reflow` function to `reflowTextForScreen` in `tt.go`.
- Updated variable names in `main` function for improved readability in `tt.go`.
- Enhanced accuracy report precision in `showReport` function in `tt.go`.
- Increased vertical line spacing in `start` function in `typer.go`.
- Added typed text display with error highlights in `start` function in `typer.go`.
- Improved text segment parameters and return types for readability in `typer.go`.
- Refactored `start` function in `typer.go` for readability and better error management.
- Added comment to `getParagraphs` function in `util.go`.
- Fixed formatting in `cell` struct in `util.go`.
- Added `.idea` to `.gitignore`.

# 0.4.2:
  Added -notheme, -blockcursor and -bold.

# 0.4.0:
  Too numerous to list (see the man page)

  Highlights:
  
 - Added -quotes.
 - Added support for navigating between tests via right/left.
 - Now store the user's position within a file if one is specified.
 - Improved documentation.

# 0.3.0:
 - Added support for custom word lists (`-words).
 - `-theme` now accepts a path.
 - Added `~/.tt/themes` and `~/.tt/words`.
 - Scrapped ~/.ttrc in favour of aliases/flags.
 - Included more default word lists. (`-list words`)

# 0.2.2:
 - Modified -g to correspond to the number of groups rather than the group size.
 - Added -multi
 - Added -v
 - Changed the default behaviour to restart the currently generated test rather than generating a new one
 - Added a CHANGELOG :P
