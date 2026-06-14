package email

import (
	"testing"
)

func testEmail() *Email {
	return &Email{
		FromName:       "John Doe",
		FromAddress:    "john@example.com",
		ToAddresses:    "alice@example.com, bob@test.com",
		Subject:        "Meeting Tomorrow at 3pm",
		BodyText:       "Hi team, let's meet tomorrow to discuss the project.",
		IsRead:         false,
		IsStarred:      true,
		HasAttachments: true,
	}
}

func TestMatchValue_Contains(t *testing.T) {
	if !matchValue("hello world", "world", MatchContains) {
		t.Error("should contain 'world'")
	}
	if matchValue("hello world", "xyz", MatchContains) {
		t.Error("should not contain 'xyz'")
	}
}

func TestMatchValue_Exact(t *testing.T) {
	if !matchValue("hello", "hello", MatchExact) {
		t.Error("exact match should work")
	}
	if matchValue("hello world", "hello", MatchExact) {
		t.Error("partial should not be exact match")
	}
}

func TestMatchValue_StartsWith(t *testing.T) {
	if !matchValue("hello world", "hello", MatchStartsWith) {
		t.Error("should start with 'hello'")
	}
	if matchValue("hello world", "world", MatchStartsWith) {
		t.Error("should not start with 'world'")
	}
}

func TestMatchValue_EndsWith(t *testing.T) {
	if !matchValue("hello world", "world", MatchEndsWith) {
		t.Error("should end with 'world'")
	}
	if matchValue("hello world", "hello", MatchEndsWith) {
		t.Error("should not end with 'hello'")
	}
}

func TestMatchValue_CaseInsensitive(t *testing.T) {
	if !matchValue("Hello World", "hello", MatchContains) {
		t.Error("matching should be case-insensitive")
	}
	if !matchValue("HELLO", "hello", MatchExact) {
		t.Error("exact match should be case-insensitive")
	}
}

func TestEvalCondition_Leaf_Subject(t *testing.T) {
	e := testEmail()
	cond := &Condition{Op: CondLeaf, Field: "subject", Value: "meeting", Match: MatchContains}
	if !EvalCondition(cond, e) {
		t.Error("subject should contain 'meeting'")
	}
}

func TestEvalCondition_Leaf_From(t *testing.T) {
	e := testEmail()
	cond := &Condition{Op: CondLeaf, Field: "from", Value: "john", Match: MatchContains}
	if !EvalCondition(cond, e) {
		t.Error("from should contain 'john'")
	}
	cond.Value = "example.com"
	if !EvalCondition(cond, e) {
		t.Error("from should contain 'example.com'")
	}
}

func TestEvalCondition_Leaf_To(t *testing.T) {
	e := testEmail()
	cond := &Condition{Op: CondLeaf, Field: "to", Value: "alice", Match: MatchContains}
	if !EvalCondition(cond, e) {
		t.Error("to should contain 'alice'")
	}
}

func TestEvalCondition_And(t *testing.T) {
	e := testEmail()
	cond := &Condition{
		Op: CondAnd,
		Children: []*Condition{
			{Op: CondLeaf, Field: "subject", Value: "meeting", Match: MatchContains},
			{Op: CondLeaf, Field: "from", Value: "john", Match: MatchContains},
		},
	}
	if !EvalCondition(cond, e) {
		t.Error("AND of two true conditions should be true")
	}

	cond.Children = append(cond.Children, &Condition{Op: CondLeaf, Field: "from", Value: "nobody", Match: MatchContains})
	if EvalCondition(cond, e) {
		t.Error("AND with one false condition should be false")
	}
}

func TestEvalCondition_Or(t *testing.T) {
	e := testEmail()
	cond := &Condition{
		Op: CondOr,
		Children: []*Condition{
			{Op: CondLeaf, Field: "from", Value: "nobody", Match: MatchContains},
			{Op: CondLeaf, Field: "subject", Value: "meeting", Match: MatchContains},
		},
	}
	if !EvalCondition(cond, e) {
		t.Error("OR with one true should be true")
	}

	cond.Children = []*Condition{
		{Op: CondLeaf, Field: "from", Value: "nobody", Match: MatchContains},
		{Op: CondLeaf, Field: "subject", Value: "xyz", Match: MatchContains},
	}
	if EvalCondition(cond, e) {
		t.Error("OR with all false should be false")
	}
}

func TestEvalCondition_Not(t *testing.T) {
	e := testEmail()
	cond := &Condition{
		Op:       CondNot,
		Children: []*Condition{{Op: CondLeaf, Field: "from", Value: "nobody", Match: MatchContains}},
	}
	if !EvalCondition(cond, e) {
		t.Error("NOT of false should be true")
	}

	cond.Children = []*Condition{{Op: CondLeaf, Field: "from", Value: "john", Match: MatchContains}}
	if EvalCondition(cond, e) {
		t.Error("NOT of true should be false")
	}
}

func TestEvalCondition_Nil(t *testing.T) {
	e := testEmail()
	if EvalCondition(nil, e) {
		t.Error("nil condition should return false")
	}
}

func TestEvalCondition_NotEmpty(t *testing.T) {
	e := testEmail()
	cond := &Condition{Op: CondNot, Children: []*Condition{}}
	if EvalCondition(cond, e) {
		t.Error("NOT with no children should return false")
	}
}

func TestEvalLeafSpecial_HasAttachment(t *testing.T) {
	e := testEmail()
	cond := &Condition{Field: "has", Value: "attachment"}
	if !EvalLeafSpecial(cond, e) {
		t.Error("email has attachments")
	}

	e.HasAttachments = false
	if EvalLeafSpecial(cond, e) {
		t.Error("email has no attachments")
	}
}

func TestEvalLeafSpecial_IsRead(t *testing.T) {
	e := testEmail()
	if EvalLeafSpecial(&Condition{Field: "is", Value: "read"}, e) {
		t.Error("email is unread")
	}
	if !EvalLeafSpecial(&Condition{Field: "is", Value: "unread"}, e) {
		t.Error("email is unread")
	}

	e.IsRead = true
	if !EvalLeafSpecial(&Condition{Field: "is", Value: "read"}, e) {
		t.Error("email should be read")
	}
}

func TestEvalLeafSpecial_IsStarred(t *testing.T) {
	e := testEmail()
	if !EvalLeafSpecial(&Condition{Field: "is", Value: "starred"}, e) {
		t.Error("email is starred")
	}

	e.IsStarred = false
	if EvalLeafSpecial(&Condition{Field: "is", Value: "starred"}, e) {
		t.Error("email is not starred")
	}
}

func TestEvalFilter_Complex(t *testing.T) {
	e := testEmail()
	pf := &ParsedFilter{
		Condition: &Condition{
			Op: CondAnd,
			Children: []*Condition{
				{Op: CondLeaf, Field: "from", Value: "john", Match: MatchContains},
				{Op: CondNot, Children: []*Condition{
					{Op: CondLeaf, Field: "subject", Value: "spam", Match: MatchContains},
				}},
			},
		},
	}
	if !EvalFilter(pf, e) {
		t.Error("from=john AND NOT subject=spam should match")
	}
}

func TestGetFieldValue_UnknownField(t *testing.T) {
	e := testEmail()
	val := getFieldValue("nonexistent", e)
	if val != "" {
		t.Errorf("unknown field should return empty, got %q", val)
	}
}
