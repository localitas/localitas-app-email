package email

import (
	"fmt"
	"strings"
	"unicode"
)

type tokenKind int

const (
	tokField  tokenKind = iota // from:, subject:, etc.
	tokValue                   // the value after field:
	tokOR                      // OR keyword
	tokNot                     // - prefix
	tokLParen                  // (
	tokRParen                  // )
	tokArrow                   // =>
	tokComma                   // ,
	tokWord                    // bare word (action)
)

type token struct {
	kind tokenKind
	text string
}

func tokenize(input string) ([]token, error) {
	var tokens []token
	runes := []rune(input)
	i := 0

	for i < len(runes) {
		ch := runes[i]

		if unicode.IsSpace(ch) {
			i++
			continue
		}

		if ch == '(' {
			tokens = append(tokens, token{kind: tokLParen, text: "("})
			i++
			continue
		}
		if ch == ')' {
			tokens = append(tokens, token{kind: tokRParen, text: ")"})
			i++
			continue
		}
		if ch == ',' {
			tokens = append(tokens, token{kind: tokComma, text: ","})
			i++
			continue
		}
		if ch == '=' && i+1 < len(runes) && runes[i+1] == '>' {
			tokens = append(tokens, token{kind: tokArrow, text: "=>"})
			i += 2
			continue
		}
		if ch == '-' {
			tokens = append(tokens, token{kind: tokNot, text: "-"})
			i++
			continue
		}

		start := i
		for i < len(runes) && !unicode.IsSpace(runes[i]) && runes[i] != '(' && runes[i] != ')' && runes[i] != ',' {
			if runes[i] == '=' && i+1 < len(runes) && runes[i+1] == '>' {
				break
			}
			i++
		}
		word := string(runes[start:i])

		if strings.ToUpper(word) == "OR" {
			tokens = append(tokens, token{kind: tokOR, text: "OR"})
			continue
		}

		colonIdx := strings.Index(word, ":")
		if colonIdx > 0 {
			field := word[:colonIdx]
			value := word[colonIdx+1:]
			modifier := ""
			if value == "" && i < len(runes) && runes[i] == '"' {
				i++
				qStart := i
				for i < len(runes) && runes[i] != '"' {
					i++
				}
				value = string(runes[qStart:i])
				if i < len(runes) {
					i++
				}
			} else if len(value) > 0 && (value[0] == '"' || ((value[0] == '^' || value[0] == '$' || value[0] == '=') && len(value) > 1 && value[1] == '"')) {
				if value[0] != '"' {
					modifier = string(value[0])
					value = value[2:]
				} else {
					value = value[1:]
				}
				if strings.HasSuffix(value, "\"") {
					value = value[:len(value)-1]
				} else {
					for i < len(runes) && runes[i] != '"' {
						value += string(runes[i])
						i++
					}
					if i < len(runes) {
						i++
					}
				}
				value = modifier + value
			}
			tokens = append(tokens, token{kind: tokField, text: field})
			tokens = append(tokens, token{kind: tokValue, text: value})
			continue
		}

		tokens = append(tokens, token{kind: tokWord, text: word})
	}

	return tokens, nil
}

type parser struct {
	tokens []token
	pos    int
}

func (p *parser) peek() *token {
	if p.pos >= len(p.tokens) {
		return nil
	}
	return &p.tokens[p.pos]
}

func (p *parser) next() *token {
	t := p.peek()
	if t != nil {
		p.pos++
	}
	return t
}

func (p *parser) expect(kind tokenKind) (*token, error) {
	t := p.next()
	if t == nil {
		return nil, fmt.Errorf("unexpected end of input, expected %d", kind)
	}
	if t.kind != kind {
		return nil, fmt.Errorf("expected token kind %d, got %d (%q)", kind, t.kind, t.text)
	}
	return t, nil
}

func ParseFilter(input string) (*ParsedFilter, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty filter rule")
	}

	tokens, err := tokenize(input)
	if err != nil {
		return nil, err
	}

	arrowIdx := -1
	for i, t := range tokens {
		if t.kind == tokArrow {
			arrowIdx = i
			break
		}
	}
	if arrowIdx < 0 {
		return nil, fmt.Errorf("missing '=>' separator between conditions and actions")
	}
	if arrowIdx == 0 {
		return nil, fmt.Errorf("no conditions before '=>'")
	}

	condTokens := tokens[:arrowIdx]
	actionTokens := tokens[arrowIdx+1:]

	if len(actionTokens) == 0 {
		return nil, fmt.Errorf("no actions after '=>'")
	}

	p := &parser{tokens: condTokens}
	cond, err := p.parseOr()
	if err != nil {
		return nil, fmt.Errorf("condition parse error: %w", err)
	}
	if p.pos < len(p.tokens) {
		return nil, fmt.Errorf("unexpected token after conditions: %q", p.tokens[p.pos].text)
	}

	actions, err := parseActions(actionTokens)
	if err != nil {
		return nil, fmt.Errorf("action parse error: %w", err)
	}

	return &ParsedFilter{Condition: cond, Actions: actions}, nil
}

func (p *parser) parseOr() (*Condition, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}

	for p.peek() != nil && p.peek().kind == tokOR {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &Condition{Op: CondOr, Children: []*Condition{left, right}}
	}

	return left, nil
}

func (p *parser) parseAnd() (*Condition, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}

	for p.peek() != nil && p.peek().kind != tokOR && p.peek().kind != tokRParen {
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &Condition{Op: CondAnd, Children: []*Condition{left, right}}
	}

	return left, nil
}

func (p *parser) parseUnary() (*Condition, error) {
	if p.peek() != nil && p.peek().kind == tokNot {
		p.next()
		child, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &Condition{Op: CondNot, Children: []*Condition{child}}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (*Condition, error) {
	t := p.peek()
	if t == nil {
		return nil, fmt.Errorf("unexpected end of input")
	}

	if t.kind == tokLParen {
		p.next()
		cond, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokRParen); err != nil {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		return cond, nil
	}

	if t.kind == tokField {
		p.next()
		valTok, err := p.expect(tokValue)
		if err != nil {
			return nil, fmt.Errorf("expected value after field %q", t.text)
		}
		field := strings.ToLower(t.text)
		value := valTok.text
		matchType := MatchContains

		if len(value) > 0 {
			switch value[0] {
			case '=':
				matchType = MatchExact
				value = value[1:]
			case '^':
				matchType = MatchStartsWith
				value = value[1:]
			case '$':
				matchType = MatchEndsWith
				value = value[1:]
			}
		}

		return &Condition{Op: CondLeaf, Field: field, Match: matchType, Value: value}, nil
	}

	if t.kind == tokWord {
		p.next()
		field := strings.ToLower(t.text)
		if strings.Contains(field, ":") {
			parts := strings.SplitN(field, ":", 2)
			return &Condition{Op: CondLeaf, Field: parts[0], Value: parts[1]}, nil
		}
		return &Condition{Op: CondLeaf, Field: field, Value: ""}, nil
	}

	return nil, fmt.Errorf("unexpected token: %q", t.text)
}

func parseActions(tokens []token) ([]Action, error) {
	var actions []Action
	var current []string

	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t.kind == tokComma {
			if len(current) > 0 {
				a, err := buildAction(strings.Join(current, " "))
				if err != nil {
					return nil, err
				}
				actions = append(actions, a)
				current = nil
			}
			continue
		}
		if t.kind == tokField && i+1 < len(tokens) && tokens[i+1].kind == tokValue {
			current = append(current, t.text+":"+tokens[i+1].text)
			i++
			continue
		}
		current = append(current, t.text)
	}
	if len(current) > 0 {
		a, err := buildAction(strings.Join(current, " "))
		if err != nil {
			return nil, err
		}
		actions = append(actions, a)
	}

	if len(actions) == 0 {
		return nil, fmt.Errorf("no actions specified")
	}
	return actions, nil
}

func buildAction(raw string) (Action, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.ToLower(raw)

	if strings.HasPrefix(raw, "move:") {
		return Action{Type: "move", Value: strings.TrimPrefix(raw, "move:")}, nil
	}
	if strings.HasPrefix(raw, "label:") {
		return Action{Type: "label", Value: strings.TrimPrefix(raw, "label:")}, nil
	}
	if strings.HasPrefix(raw, "forward:") {
		return Action{Type: "forward", Value: strings.TrimPrefix(raw, "forward:")}, nil
	}

	switch raw {
	case "archive":
		return Action{Type: "archive"}, nil
	case "delete":
		return Action{Type: "delete"}, nil
	case "star":
		return Action{Type: "star"}, nil
	case "mark read":
		return Action{Type: "mark_read"}, nil
	case "mark unread":
		return Action{Type: "mark_unread"}, nil
	default:
		return Action{Type: raw}, nil
	}
}
