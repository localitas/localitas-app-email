package email

import (
	"strings"
)

func EvalCondition(cond *Condition, e *Email) bool {
	if cond == nil {
		return false
	}

	switch cond.Op {
	case CondAnd:
		for _, child := range cond.Children {
			if !EvalCondition(child, e) {
				return false
			}
		}
		return true

	case CondOr:
		for _, child := range cond.Children {
			if EvalCondition(child, e) {
				return true
			}
		}
		return false

	case CondNot:
		if len(cond.Children) == 0 {
			return false
		}
		return !EvalCondition(cond.Children[0], e)

	case CondLeaf:
		return evalLeaf(cond, e)
	}

	return false
}

func evalLeaf(cond *Condition, e *Email) bool {
	fieldValue := getFieldValue(cond.Field, e)
	return matchValue(fieldValue, cond.Value, cond.Match)
}

func getFieldValue(field string, e *Email) string {
	switch field {
	case "from":
		return e.FromName + " " + e.FromAddress
	case "to":
		return e.ToAddresses
	case "subject":
		return e.Subject
	case "body":
		return e.BodyText
	case "has":
		return ""
	case "is":
		return ""
	default:
		return ""
	}
}

func matchValue(fieldValue, pattern string, matchType MatchType) bool {
	fv := strings.ToLower(fieldValue)
	p := strings.ToLower(pattern)

	switch matchType {
	case MatchExact:
		return fv == p
	case MatchStartsWith:
		return strings.HasPrefix(fv, p)
	case MatchEndsWith:
		return strings.HasSuffix(fv, p)
	case MatchContains:
		return strings.Contains(fv, p)
	}
	return false
}

func EvalLeafSpecial(cond *Condition, e *Email) bool {
	switch cond.Field {
	case "has":
		switch strings.ToLower(cond.Value) {
		case "attachment", "attachments":
			return e.HasAttachments
		}
	case "is":
		switch strings.ToLower(cond.Value) {
		case "unread":
			return !e.IsRead
		case "read":
			return e.IsRead
		case "starred":
			return e.IsStarred
		}
	}
	return false
}

func evalLeafFull(cond *Condition, e *Email) bool {
	if cond.Field == "has" || cond.Field == "is" {
		return EvalLeafSpecial(cond, e)
	}
	return evalLeaf(cond, e)
}

func EvalFilter(pf *ParsedFilter, e *Email) bool {
	return evalConditionFull(pf.Condition, e)
}

func evalConditionFull(cond *Condition, e *Email) bool {
	if cond == nil {
		return false
	}

	switch cond.Op {
	case CondAnd:
		for _, child := range cond.Children {
			if !evalConditionFull(child, e) {
				return false
			}
		}
		return true

	case CondOr:
		for _, child := range cond.Children {
			if evalConditionFull(child, e) {
				return true
			}
		}
		return false

	case CondNot:
		if len(cond.Children) == 0 {
			return false
		}
		return !evalConditionFull(cond.Children[0], e)

	case CondLeaf:
		return evalLeafFull(cond, e)
	}

	return false
}

func ApplyFilters(filters []*ParsedFilter, e *Email) []Action {
	var actions []Action
	for _, f := range filters {
		if EvalFilter(f, e) {
			actions = append(actions, f.Actions...)
		}
	}
	return actions
}
