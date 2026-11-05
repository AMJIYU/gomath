package code

import (
	"fmt"
	"go/scanner"
	"go/token"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Line describes an original byte span excluding its LF or CRLF terminator.
// Indices and columns are zero based; Number is the displayed decimal label.
// Columns includes leading/trailing spaces and expanded tabs. A trailing newline
// creates a final empty line; empty source has one empty line.
type Line struct{ Index, Start, End, Columns, Number int }

// Token spans original UTF-8 bytes, including CR in multiline raw literals or
// comments. Text is verbatim source. Line/Column are physical glyph-grid
// coordinates, unaffected by //line directives. Inserted semicolons are omitted.
type Token struct {
	Index, Start, End, Line, Column int
	Category                        Category
	Text                            string
}

// SourceError reports a lexical error at an original byte offset. Line/Column
// are zero-based physical rune-cell coordinates (tabs expanded).
type SourceError struct {
	Offset, Line, Column int
	Message              string
}

func (e *SourceError) Error() string {
	return fmt.Sprintf("%v at byte %d (line %d, cell %d): %s", ErrSource, e.Offset, e.Line, e.Column, e.Message)
}
func (e *SourceError) Unwrap() error { return ErrSource }

type cell struct {
	r                           rune
	offset, line, column, token int
}

func inspect(source string, c config) ([]Line, []cell, error) {
	if len(source) > c.limits.SourceBytes {
		return nil, nil, fmt.Errorf("%w: source bytes > %d", ErrLimit, c.limits.SourceBytes)
	}
	if !utf8.ValidString(source) {
		return nil, nil, fmt.Errorf("%w: source is not UTF-8", ErrSource)
	}
	if utf8.RuneCountInString(source) > c.limits.Runes {
		return nil, nil, fmt.Errorf("%w: source runes > %d", ErrLimit, c.limits.Runes)
	}
	var lines []Line
	var cells []cell
	start, column, glyphs := 0, 0, 0
	finish := func(end, next int) error {
		if len(lines) >= c.limits.Lines {
			return fmt.Errorf("%w: lines > %d", ErrLimit, c.limits.Lines)
		}
		lines = append(lines, Line{len(lines), start, end, column, c.FirstLineNumber + len(lines)})
		start, column = next, 0
		return nil
	}
	for offset := 0; offset < len(source); {
		r, n := utf8.DecodeRuneInString(source[offset:])
		if r == '\r' {
			if offset+1 >= len(source) || source[offset+1] != '\n' {
				return nil, nil, &SourceError{offset, len(lines), column, "bare CR is unsupported; use LF or CRLF"}
			}
			if err := finish(offset, offset+2); err != nil {
				return nil, nil, err
			}
			offset += 2
			continue
		}
		if r == '\n' {
			if err := finish(offset, offset+1); err != nil {
				return nil, nil, err
			}
			offset += n
			continue
		}
		if r != '\t' && (unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.M, unicode.Zl, unicode.Zp) || unicode.IsSpace(r) && r != ' ') {
			return nil, nil, &SourceError{offset, len(lines), column, "unsupported control, format, combining or non-horizontal whitespace character"}
		}
		advance := 1
		if r == '\t' {
			advance = c.TabWidth - column%c.TabWidth
		}
		if column+advance > c.limits.Columns {
			return nil, nil, fmt.Errorf("%w: columns > %d on line %d", ErrLimit, c.limits.Columns, len(lines))
		}
		if r != ' ' && r != '\t' {
			glyphs++
			if glyphs > c.limits.Glyphs {
				return nil, nil, fmt.Errorf("%w: glyphs > %d", ErrLimit, c.limits.Glyphs)
			}
			cells = append(cells, cell{r, offset, len(lines), column, -1})
		}
		column += advance
		offset += n
	}
	if err := finish(len(source), len(source)); err != nil {
		return nil, nil, err
	}
	return lines, cells, nil
}

func location(source string, lines []Line, offset, tabWidth int) (int, int) {
	// Scanner positions can be in a terminator; retain the preceding line.
	lo, hi := 0, len(lines)
	for lo+1 < hi {
		mid := (lo + hi) / 2
		if lines[mid].Start <= offset {
			lo = mid
		} else {
			hi = mid
		}
	}
	col := 0
	for _, r := range source[lines[lo].Start:min(offset, lines[lo].End)] {
		if r == '\t' {
			col += tabWidth - col%tabWidth
		} else {
			col++
		}
	}
	return lo, col
}

func lex(source string, lines []Line, c config) ([]Token, error) {
	file := token.NewFileSet().AddFile("", -1, len(source))
	var scan scanner.Scanner
	var first *SourceError
	scan.Init(file, []byte(source), func(p token.Position, msg string) {
		if first == nil {
			line, col := location(source, lines, p.Offset, c.TabWidth)
			first = &SourceError{p.Offset, line, col, msg}
		}
	}, scanner.ScanComments)
	var out []Token
	depth := 0
	for {
		pos, tok, lit := scan.Scan()
		if first != nil {
			return nil, first
		}
		if tok == token.EOF {
			return out, nil
		}
		if tok == token.SEMICOLON && lit == "\n" {
			continue
		}
		if len(out) >= c.limits.Tokens {
			return nil, fmt.Errorf("%w: tokens > %d", ErrLimit, c.limits.Tokens)
		}
		if tok == token.LPAREN || tok == token.LBRACK || tok == token.LBRACE {
			depth++
			if depth > c.limits.Depth {
				return nil, fmt.Errorf("%w: lexical delimiter depth > %d", ErrLimit, c.limits.Depth)
			}
		} else if tok == token.RPAREN || tok == token.RBRACK || tok == token.RBRACE {
			depth = max(0, depth-1)
		}
		start := file.Offset(pos)
		end := start + len(lit)
		category := Identifier
		switch {
		case tok.IsKeyword():
			category = Keyword
		case tok == token.IDENT:
		case tok == token.INT || tok == token.FLOAT || tok == token.IMAG:
			category = Number
		case tok == token.STRING || tok == token.CHAR:
			category = String
			// go/scanner strips CR in raw literals. Recover delimiters from
			// original source rather than deriving an end from stripped lit.
			if source[start] == '`' {
				end = start + 1 + strings.IndexByte(source[start+1:], '`') + 1
			}
		case tok == token.COMMENT:
			category = Comment
			if source[start+1] == '/' {
				end = len(source)
				if i := strings.IndexByte(source[start:], '\n'); i >= 0 {
					end = start + i
					if end > start && source[end-1] == '\r' {
						end--
					}
				}
			} else {
				end = start + 2 + strings.Index(source[start+2:], "*/") + 2
			}
		default:
			end = start + len(tok.String())
			category = Operator
			switch tok {
			case token.LPAREN, token.RPAREN, token.LBRACK, token.RBRACK, token.LBRACE, token.RBRACE, token.COMMA, token.PERIOD, token.SEMICOLON, token.COLON:
				category = Punctuation
			}
		}
		if end <= start || end > len(source) {
			return nil, fmt.Errorf("%w: scanner span outside source", ErrSource)
		}
		line, col := location(source, lines, start, c.TabWidth)
		out = append(out, Token{len(out), start, end, line, col, category, source[start:end]})
	}
}
