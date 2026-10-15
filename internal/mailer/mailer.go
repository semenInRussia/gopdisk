package mailer

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	texttemplate "text/template"
	"time"

	mail "github.com/go-mail/mail/v2"
)

//go:embed templates
var templateFS embed.FS

type Mailer struct {
	dialer *mail.Dialer
	sender string
}

func New(host string, port int, username, password, sender string) *Mailer {
	dialer := mail.NewDialer(host, port, username, password)
	dialer.Timeout = 5 * time.Second
	return &Mailer{dialer: dialer, sender: sender}
}

func (m *Mailer) Send(recipient, templateFile string, data any) error {
	path := "templates/" + templateFile
	textTemplates, err := texttemplate.New("mail").ParseFS(templateFS, path)
	if err != nil {
		return err
	}

	var subject bytes.Buffer
	if err := textTemplates.ExecuteTemplate(&subject, "subject", data); err != nil {
		return err
	}
	var plainBody bytes.Buffer
	if err := textTemplates.ExecuteTemplate(&plainBody, "plainBody", data); err != nil {
		return err
	}
	htmlTemplates, err := htmltemplate.New("mail").ParseFS(templateFS, path)
	if err != nil {
		return err
	}
	var htmlBody bytes.Buffer
	if err := htmlTemplates.ExecuteTemplate(&htmlBody, "htmlBody", data); err != nil {
		return err
	}

	message := mail.NewMessage()
	message.SetHeader("To", recipient)
	message.SetHeader("From", m.sender)
	message.SetHeader("Subject", subject.String())
	message.SetBody("text/plain", plainBody.String())
	message.AddAlternative("text/html", htmlBody.String())

	if err := m.dialer.DialAndSend(message); err != nil {
		return fmt.Errorf("send mail: %w", err)
	}
	return nil
}
