package mailer

import (
	"bytes"
	htmltemplate "html/template"
	"strings"
	"testing"
	texttemplate "text/template"
)

func TestWelcomeTemplate(t *testing.T) {
	const link = "https://gopherdisk.online/activation?token=ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	data := map[string]any{"user": struct{ Name string }{"<Alice>"}, "activationURL": link}
	plain, err := texttemplate.ParseFS(templateFS, "templates/user_welcome.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	html, err := htmltemplate.ParseFS(templateFS, "templates/user_welcome.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"plainBody", "htmlBody"} {
		t.Run(name, func(t *testing.T) {
			var body bytes.Buffer
			var err error
			if name == "plainBody" {
				err = plain.ExecuteTemplate(&body, name, data)
			} else {
				err = html.ExecuteTemplate(&body, name, data)
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{link, "3 дня", "не активирует аккаунт", "проигнорируйте"} {
				if !strings.Contains(body.String(), want) {
					t.Errorf("missing %q in %s", want, body.String())
				}
			}
			if name == "htmlBody" && (strings.Contains(body.String(), "<Alice>") || !strings.Contains(body.String(), `href="`+link+`"`)) {
				t.Errorf("incorrect HTML escaping or link: %s", body.String())
			}
		})
	}
}
