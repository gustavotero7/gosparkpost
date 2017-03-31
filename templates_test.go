package gosparkpost_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	sp "github.com/SparkPost/gosparkpost"
	"github.com/pkg/errors"
)

var templateFromValidationTests = []struct {
	in  interface{}
	err error
	out sp.From
}{
	{sp.From{"a@b.com", "A B"}, nil, sp.From{"a@b.com", "A B"}},
	{sp.Address{"a@b.com", "A B", "c@d.com"}, nil, sp.From{"a@b.com", "A B"}},
	{"a@b.com", nil, sp.From{"a@b.com", ""}},
	{nil, errors.New("unsupported Content.From value type [%!s(<nil>)]"), sp.From{"", ""}},
	{[]byte("a@b.com"), errors.New("unsupported Content.From value type [[]uint8]"), sp.From{"", ""}},
	{"", errors.New("Content.From may not be empty"), sp.From{"", ""}},
	{map[string]interface{}{"name": "A B", "email": "a@b.com"}, nil, sp.From{"a@b.com", "A B"}},
	{map[string]interface{}{"name": 1, "email": "a@b.com"}, errors.New("strings are required for all Content.From values"),
		sp.From{"a@b.com", ""}},
	{map[string]string{"name": "A B", "email": "a@b.com"}, nil, sp.From{"a@b.com", "A B"}},
}

func TestTemplateFromValidation(t *testing.T) {
	for idx, test := range templateFromValidationTests {
		f, err := sp.ParseFrom(test.in)
		if err == nil && test.err != nil || err != nil && test.err == nil {
			t.Errorf("ParseFrom[%d] => err %q, want %q", idx, err, test.err)
		} else if err != nil && err.Error() != test.err.Error() {
			t.Errorf("ParseFrom[%d] => err %q, want %q", idx, err, test.err)
		} else if f.Email != test.out.Email {
			t.Errorf("ParseFrom[%d] => Email %q, want %q", idx, f.Email, test.out.Email)
		} else if f.Name != test.out.Name {
			t.Errorf("ParseFrom[%d] => Name %q, want %q", idx, f.Name, test.out.Name)
		}
	}
}

var templateValidationTests = []struct {
	te  *sp.Template
	err error
	cmp func(te *sp.Template) bool
}{
	{nil, errors.New("Can't Validate a nil Template"), nil},
	{&sp.Template{}, errors.New("Template requires a non-empty Content.Subject"), nil},
	{&sp.Template{Content: sp.Content{Subject: "s"}}, errors.New("Template requires either Content.HTML or Content.Text"), nil},
	{&sp.Template{Content: sp.Content{Subject: "s", HTML: "h", From: ""}},
		errors.New("Content.From may not be empty"), nil},

	{&sp.Template{ID: strings.Repeat("id", 33), Content: sp.Content{Subject: "s", HTML: "h", From: "f"}},
		errors.New("Template id may not be longer than 64 bytes"), nil},
	{&sp.Template{Name: strings.Repeat("name", 257), Content: sp.Content{Subject: "s", HTML: "h", From: "f"}},
		errors.New("Template name may not be longer than 1024 bytes"), nil},
	{&sp.Template{Description: strings.Repeat("desc", 257), Content: sp.Content{Subject: "s", HTML: "h", From: "f"}},
		errors.New("Template description may not be longer than 1024 bytes"), nil},

	{&sp.Template{
		Content: sp.Content{
			Subject: "s", HTML: "h", From: "f",
			Attachments: []sp.Attachment{{Filename: strings.Repeat("f", 256)}},
		}},
		errors.Errorf("Attachment name length must be <= 255: [%s]", strings.Repeat("f", 256)), nil},
	{&sp.Template{
		Content: sp.Content{
			Subject: "s", HTML: "h", From: "f",
			Attachments: []sp.Attachment{{B64Data: "\r\n"}},
		}},
		errors.New("Attachment data may not contain line breaks [\\r\\n]"), nil},

	{&sp.Template{
		Content: sp.Content{
			Subject: "s", HTML: "h", From: "f",
			InlineImages: []sp.InlineImage{{Filename: strings.Repeat("f", 256)}},
		}},
		errors.Errorf("InlineImage name length must be <= 255: [%s]", strings.Repeat("f", 256)), nil},
	{&sp.Template{
		Content: sp.Content{
			Subject: "s", HTML: "h", From: "f",
			InlineImages: []sp.InlineImage{{B64Data: "\r\n"}},
		}},
		errors.New("InlineImage data may not contain line breaks [\\r\\n]"), nil},

	{
		&sp.Template{Content: sp.Content{EmailRFC822: "From:foo@example.com\r\n", Subject: "removeme"}},
		nil,
		func(te *sp.Template) bool { return te.Content.Subject == "" },
	},
}

func TestTemplateValidation(t *testing.T) {
	for idx, test := range templateValidationTests {
		err := test.te.Validate()
		if err == nil && test.err != nil || err != nil && test.err == nil {
			t.Errorf("Template.Validate[%d] => err %q, want %q", idx, err, test.err)
		} else if err != nil && err.Error() != test.err.Error() {
			t.Errorf("Template.Validate[%d] => err %q, want %q", idx, err, test.err)
		} else if test.cmp != nil && test.cmp(test.te) == false {
			t.Errorf("Template.Validate[%d] => failed post-condition check for %q", test.te)
		}
	}
}

var templatePostSuccessTests = []struct {
	in     *sp.Template
	err    error
	status int
	json   string
	id     string
}{
	{nil, errors.New("Create called with nil Template"), 0, "", ""},
	{&sp.Template{}, errors.New("Template requires a non-empty Content.Subject"), 0, "", ""},
	{&sp.Template{Content: sp.Content{Subject: "s", HTML: "h", From: "f"}},
		errors.New("Unexpected response to Template creation (results)"),
		200, `{"foo":{"id":"new-template"}}`, ""},
	{&sp.Template{Content: sp.Content{Subject: "s", HTML: "h", From: "f"}},
		errors.New("Unexpected response to Template creation"),
		200, `{"results":{"ID":"new-template"}}`, ""},

	{&sp.Template{Content: sp.Content{Subject: "s{{", HTML: "h", From: "f"}},
		errors.New("3000: substitution language syntax error in template content\nError while compiling header Subject: substitution statement missing ending '}}'"),
		422, `{ "errors": [ { "message": "substitution language syntax error in template content", "description": "Error while compiling header Subject: substitution statement missing ending '}}'", "code": "3000", "part": "Header:Subject" } ] }`, ""},

	{&sp.Template{Content: sp.Content{Subject: "s", HTML: "h", From: "f"}}, nil,
		200, `{"results":{"id":"new-template"}}`, "new-template"},
}

func TestTemplateCreate(t *testing.T) {
	for idx, test := range templatePostSuccessTests {
		testSetup(t)
		defer testTeardown()

		path := fmt.Sprintf(sp.TemplatesPathFormat, testClient.Config.ApiVersion)
		testMux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			testMethod(t, r, "POST")
			w.Header().Set("Content-Type", "application/json; charset=utf8")
			w.WriteHeader(test.status)
			w.Write([]byte(test.json))
		})

		id, _, err := testClient.TemplateCreate(test.in)
		if err == nil && test.err != nil || err != nil && test.err == nil {
			t.Errorf("TemplateCreate[%d] => err %q want %q", idx, err, test.err)
		} else if err != nil && err.Error() != test.err.Error() {
			t.Errorf("TemplateCreate[%d] => err %q want %q", idx, err, test.err)
		} else if id != test.id {
			t.Errorf("TemplateCreate[%d] => id %q want %q", idx, id, test.id)
		}
	}
}
