// Package theme defines the colors of the interface.
//
// A color is a string: "#rrggbb" for true color, "0" to "255" for the
// terminal's own palette, or "" for the terminal's default. Bubble Tea
// converts true colors down to 256 or 16 colors when the terminal cannot
// show them.
package theme

// Syntax holds the colors of each syntax class.
type Syntax struct {
	Keyword  string `json:"keyword"`
	Type     string `json:"type"`
	Function string `json:"function"`
	String   string `json:"string"`
	Number   string `json:"number"`
	Comment  string `json:"comment"`
	Operator string `json:"operator"`
}

// Theme is a complete set of interface colors.
type Theme struct {
	Name string `json:"name"`
	Dark bool   `json:"dark"` // used to pick a default for the terminal

	Background string `json:"background"`
	Foreground string `json:"foreground"`
	Muted      string `json:"muted"`  // hints, hidden-file notes
	Header     string `json:"header"` // titles and the active range
	Border     string `json:"border"` // separators between panes and rows

	LineNumber string `json:"line_number"`
	Selection  string `json:"selection"` // selected row in the file list
	Cursor     string `json:"cursor"`    // line numbers of the current change
	Filler     string `json:"filler"`    // empty side of an added/removed row

	AddedBg     string `json:"added_bg"`
	RemovedBg   string `json:"removed_bg"`
	AddedWord   string `json:"added_word"`   // changed words in an added line
	RemovedWord string `json:"removed_word"` // changed words in a removed line
	AddedFg     string `json:"added_fg"`     // "+" signs and added counts
	RemovedFg   string `json:"removed_fg"`   // "-" signs and removed counts
	ModifiedFg  string `json:"modified_fg"`  // modified counts

	Syntax Syntax `json:"syntax"`
}

// Builtins returns the built-in themes, in the order "t" cycles them.
func Builtins() []Theme {
	return []Theme{dark(), light(), highContrast(), ansi()}
}

// dark uses the terminal palette from ~/.bashrc: lavender text on near
// black, mint for additions, pink for removals. Change backgrounds are
// the mint and pink mixed into the background (12% for lines, 28% for
// changed words) so syntax colors stay readable on top of them.
func dark() Theme {
	return Theme{
		Name: "dark", Dark: true,
		Background: "#0c0c0c", Foreground: "#b7bdf8", Muted: "#5b5e7c",
		Header: "#ffffff", Border: "#2b2d3f",
		LineNumber: "#5b5e7c", Selection: "#26264d", Cursor: "#fffa8c", Filler: "#141418",
		AddedBg: "#152622", RemovedBg: "#291b25",
		AddedWord: "#204840", RemovedWord: "#502f46",
		AddedFg: "#54e2c5", RemovedFg: "#ff89dc", ModifiedFg: "#fffa8c",
		Syntax: Syntax{
			Keyword: "#ff89dc", Type: "#7a73db", Function: "#fffa8c",
			String: "#54e2c5", Number: "#7a73db", Comment: "#767676", Operator: "#b7bdf8",
		},
	}
}

func light() Theme {
	return Theme{
		Name: "light", Dark: false,
		Background: "#fafafa", Foreground: "#383a42", Muted: "#8e9099",
		Header: "#4078f2", Border: "#d4d4d8",
		LineNumber: "#a0a1a7", Selection: "#e4e7ec", Cursor: "#c18401", Filler: "#efefef",
		AddedBg: "#e3f4e6", RemovedBg: "#fbe7e9",
		AddedWord: "#a8dcb5", RemovedWord: "#f2b1b9",
		AddedFg: "#2e7d32", RemovedFg: "#c62828", ModifiedFg: "#a35f00",
		Syntax: Syntax{
			Keyword: "#a626a4", Type: "#a35f00", Function: "#4078f2",
			String: "#3f8f3e", Number: "#986801", Comment: "#9a9ba1", Operator: "#0184bc",
		},
	}
}

func highContrast() Theme {
	return Theme{
		Name: "high-contrast", Dark: true,
		Background: "#000000", Foreground: "#ffffff", Muted: "#c0c0c0",
		Header: "#ffff00", Border: "#ffffff",
		LineNumber: "#b0b0b0", Selection: "#303080", Cursor: "#ffff00", Filler: "#141414",
		AddedBg: "#003a00", RemovedBg: "#420000",
		AddedWord: "#008a00", RemovedWord: "#b00000",
		AddedFg: "#33ff33", RemovedFg: "#ff5555", ModifiedFg: "#ffff00",
		Syntax: Syntax{
			Keyword: "#ff80ff", Type: "#ffff66", Function: "#66ffff",
			String: "#66ff66", Number: "#ffb000", Comment: "#b0b0b0", Operator: "#ffffff",
		},
	}
}

// ansi uses the terminal's 16 colors for text, so it follows whatever
// color scheme the terminal has, and fixed 256-palette colors for the
// change backgrounds. It never sets the background.
func ansi() Theme {
	return Theme{
		Name: "ansi", Dark: true,
		Background: "", Foreground: "", Muted: "8",
		Header: "4", Border: "8",
		LineNumber: "8", Selection: "237", Cursor: "3", Filler: "235",
		AddedBg: "22", RemovedBg: "52",
		AddedWord: "28", RemovedWord: "124",
		AddedFg: "2", RemovedFg: "1", ModifiedFg: "3",
		Syntax: Syntax{
			Keyword: "5", Type: "3", Function: "4",
			String: "2", Number: "11", Comment: "8", Operator: "6",
		},
	}
}
