package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/apex/log"
	"github.com/apex/log/handlers/cli"
	"github.com/microcosm-cc/bluemonday"
	sendgrid "github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/vanng822/go-premailer/premailer"
	"golang.org/x/oauth2/google"
	drive "google.golang.org/api/drive/v3"
)

// Notes:
// Inline styles: https://github.com/vanng822/go-premailer

var (
	fileID           = flag.String("file-id", "", "Google Docs file ID")
	sendgridAPIKey   = flag.String("sendgrid-api-key", "", "Sendgrid API key")
	emailFromName    = flag.String("email-from-name", "", "Name of recipient")
	emailFromAddress = flag.String("email-from-address", "", "Address of recipient")
	emailToName      = flag.String("email-to-name", "", "Name of recipient")
	emailToAddress   = flag.String("email-to-address", "", "Address of recipient")
	emailSubject     = flag.String("email-subject", "", "Subject line")

	reStyleAttr, _ = regexp.Compile("style=\"[^\"]+\"")

	validStyles = map[string]bool{
		"font-style:italic": true,
		"font-weight:700":   true,
	}
)

func main() {
	flag.Parse()
	log.SetHandler(cli.New(os.Stderr))

	if *sendgridAPIKey == "" {
		log.Fatal("Sendgrid not configured. Please set SENDGRID_API_KEY")
	}

	clientID, err := ioutil.ReadFile("client_id.json")
	if err != nil {
		log.WithError(err).Error("Unable to read client_id.json.")
		log.Error("Request credentials from https://console.developers.google.com/start/api?id=drive")
		log.Fatal("Request access to both Gmail and Drive for your project.")
	}

	// Credentials are cached in ~/.credentials/drive2email.json
	config, err := google.ConfigFromJSON(clientID, drive.DriveReadonlyScope)
	if err != nil {
		log.WithError(err).Fatal("Unable to parse client secret file to config")
	}
	client := getClient(context.Background(), config)

	// Request a zip file export from Google Drive for the doc.

	driveService, err := drive.New(client)
	if err != nil {
		log.WithError(err).Fatal("Unable to create Drive Client")
	}

	log.Info("Requesting file")
	resp, err := driveService.Files.Export(*fileID, "application/zip").Download()
	if err != nil {
		log.WithError(err).Fatal("Failed to retrieve file")
	}
	defer resp.Body.Close()

	log.Info("Parsing zip file")
	z, err := readZip(resp.Body)
	if err != nil {
		log.WithError(err).Fatal("Failed to read zip")
	}
	filename, files := parseZipFile(z)
	if filename == "" {
		log.Fatal("Zip file does not contain a HTML file")
	}

	log.WithField("filename", filename).Info("HTML file found")

	// Clean up the HTML that Google sends us.
	sanitizer := bluemonday.UGCPolicy()
	sanitizer.AllowStyling()
	sanitizer.AllowAttrs("style").OnElements("span")
	htmlContent := string(sanitizer.SanitizeBytes(files[filename]))

	// HackHack: Google doesn't use markup for basic styling so remove all the
	// junk but keep the parts responsible for bold and italic etc.
	htmlContent = reStyleAttr.ReplaceAllStringFunc(htmlContent, func(styles string) string {
		styles = strings.TrimPrefix(styles, "style=\"")
		styles = strings.TrimSuffix(styles, "\"")
		in := strings.Split(styles, ";")
		out := []string{}
		for _, s := range in {
			if ok := validStyles[s]; ok {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return ""
		}
		return fmt.Sprintf("style=\"%s\"", strings.Join(out, ";"))
	})

	for name := range files {
		htmlContent = strings.Replace(htmlContent, name, "cid:"+name, -1)
	}

	// Inline additional styles
	p := premailer.NewPremailerFromString(htmlHeader+htmlContent+htmlFooter, premailer.NewOptions())
	styledContent, err := p.Transform()
	if err != nil {
		log.WithError(err).Fatal("Failed inlining styles")
	}

	if err := ioutil.WriteFile(filename, []byte(styledContent), os.ModePerm); err != nil {
		log.WithError(err).WithField("filename", filename).Error("Failed to write debug file, continuing")
	} else {
		log.WithField("filename", filename).Info("Writing debug file")
	}

	// Create the email and send via Sendgrid.

	from := mail.NewEmail(*emailFromName, *emailFromAddress)
	to := mail.NewEmail(*emailToName, *emailToAddress)
	subject := *emailSubject

	message := mail.NewSingleEmail(from, subject, to, "[Please enable HTML emails]", styledContent)
	for name, body := range files {
		if name != filename {
			a := mail.NewAttachment()
			a.SetContent(base64.StdEncoding.EncodeToString(body))
			a.SetType(http.DetectContentType(body))
			a.SetFilename(name)
			a.SetDisposition("inline")
			a.SetContentID(name)
			message.AddAttachment(a)
		}
	}

	sg := sendgrid.NewSendClient(*sendgridAPIKey)
	if _, err := sg.Send(message); err != nil {
		log.WithError(err).Fatal("Failed to send email")
	}

	log.Info("Email SENT!!")
}
