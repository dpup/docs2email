package main

import (
	"context"
	"encoding/base64"
	"flag"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/apex/log"
	"github.com/apex/log/handlers/cli"
	sendgrid "github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"golang.org/x/oauth2/google"
	drive "google.golang.org/api/drive/v3"
)

// TODO:
// - Send test email
// - Wait for approval
// - Send to mutliple recipients? or just use a google group.
// - Reply-To
// - BCC
// - Config file?

var (
	fileID           = flag.String("file-id", "", "Google Docs file ID")
	sendgridAPIKey   = flag.String("sendgrid-api-key", "", "Sendgrid API key")
	emailFromName    = flag.String("email-from-name", "", "Name of recipient")
	emailFromAddress = flag.String("email-from-address", "", "Address of recipient")
	emailToName      = flag.String("email-to-name", "", "Name of recipient")
	emailToAddress   = flag.String("email-to-address", "", "Address of recipient")
	emailSubject     = flag.String("email-subject", "", "Subject line")
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

	htmlContent, err := cleanHTML(string(files[filename]))
	if err != nil {
		log.WithError(err).Fatal("Failed to clean HTML")
	}

	// Look for any URLs that reference files within the archive, and update their
	// URL to look at the email "cid" scheme.
	for name := range files {
		htmlContent = strings.Replace(htmlContent, name, "cid:"+name, -1)
	}

	if err := ioutil.WriteFile(filename, []byte(htmlContent), os.ModePerm); err != nil {
		log.WithError(err).WithField("filename", filename).Error("Failed to write debug file, continuing")
	} else {
		log.WithField("filename", filename).Info("Writing debug file")
	}

	// Create the email and send via Sendgrid.

	from := mail.NewEmail(*emailFromName, *emailFromAddress)
	to := mail.NewEmail(*emailToName, *emailToAddress)
	subject := *emailSubject

	log.WithField("subject", subject).Info("Preparing email")

	m := mail.NewV3Mail()
	m.SetFrom(from)
	m.Subject = subject

	p := mail.NewPersonalization()
	p.AddTos(to)
	m.AddPersonalizations(p)

	c := mail.NewContent("text/html", htmlContent)
	m.AddContent(c)

	for name, body := range files {
		if name != filename {
			a := mail.NewAttachment()
			a.SetContent(base64.StdEncoding.EncodeToString(body))
			a.SetType(http.DetectContentType(body))
			a.SetFilename(name)
			a.SetDisposition("inline")
			a.SetContentID(name)
			m.AddAttachment(a)
		}
	}

	return

	sg := sendgrid.NewSendClient(*sendgridAPIKey)
	if _, err := sg.Send(m); err != nil {
		log.WithError(err).Fatal("Failed to send email")
	}

	log.Info("Email SENT!!")
}
