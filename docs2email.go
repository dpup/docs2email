package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	netmail "net/mail"
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
// - Config file?

var (
	fileID         = flag.String("file-id", "", "Google Docs file ID")
	sendgridAPIKey = flag.String("sendgrid-api-key", "", "Sendgrid API key")

	emailFrom    = flag.String("from", "", "Sender: e.g. Alice <alice@example.com>")
	emailTest    = flag.String("test", "", "Test recipient: e.g. Alice <alice@example.com>")
	emailTo      = flag.String("to", "", "Recipient list: e.g. Alice <alice@example.com>, Bob <bob@example.com>, Eve <eve@example.com>")
	emailCC      = flag.String("cc", "", "CC list: e.g. Alice <alice@example.com>, Bob <bob@example.com>, Eve <eve@example.com>")
	emailBCC     = flag.String("bcc", "", "BCC list: e.g. Alice <alice@example.com>, Bob <bob@example.com>, Eve <eve@example.com>")
	emailSubject = flag.String("subject", "", "Subject line")
)

func main() {
	flag.Parse()
	log.SetHandler(cli.New(os.Stderr))

	if *sendgridAPIKey == "" {
		log.Fatal("Sendgrid not configured. Please set SENDGRID_API_KEY")
	}
	if *emailSubject == "" {
		log.Fatal("No subject specified")
	}
	sender, err := netmail.ParseAddress(*emailFrom)
	if err != nil {
		log.WithError(err).Fatal("Failed to parse sender")
	}
	testRecipient, err := netmail.ParseAddress(*emailTest)
	if err != nil {
		log.WithError(err).Fatal("Failed to parse test recipient")
	}
	to, err := parseAddressList(*emailTo)
	if err != nil {
		log.WithError(err).Fatal("Failed to parse recipient list")
	}
	cc, err := parseAddressList(*emailCC)
	if err != nil {
		log.WithError(err).Fatal("Failed to parse CC list")
	}
	bcc, err := parseAddressList(*emailBCC)
	if err != nil {
		log.WithError(err).Fatal("Failed to parse BCC list")
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

	log.WithField("filename", filename).Info("Cleaning and styling HTML")
	htmlContent, err := cleanHTML(string(files[filename]))
	if err != nil {
		log.WithError(err).Fatal("Failed to clean HTML")
	}

	// Look for any URLs that reference files within the archive, and update their
	// URL to look at the email "cid" scheme.
	for name := range files {
		htmlContent = strings.Replace(htmlContent, name, "cid:"+name, -1)
	}

	log.WithField("filename", filename).Info("Writing debug file")
	if err := ioutil.WriteFile(filename, []byte(htmlContent), os.ModePerm); err != nil {
		log.WithError(err).WithField("filename", filename).Error("Failed to write debug file, continuing")
	}

	log.WithFields(log.Fields{"to": to, "cc": cc, "bcc": bcc}).Info("Reading recipients")

	sg := sendgrid.NewSendClient(*sendgridAPIKey)

	// Create an email and send it to the test account.
	log.Info("Preparing test email")
	testMail := constructEmail(sender, *emailSubject, htmlContent, filename, files)
	p := mail.NewPersonalization()
	p.AddTos(mail.NewEmail(testRecipient.Name, testRecipient.Address))
	testMail.AddPersonalizations(p)
	log.Infof("Sending test email to %s", testRecipient.Address)
	if _, err := sg.Send(testMail); err != nil {
		log.WithError(err).Fatal("Failed to send email")
	}
	log.Info("Test email sent, check your inbox")

	// TODO: Wait for user input.

	if !confirm("Send real email?") {
		log.Info("Ok, exiting. No email was sent!")
		return
	}

	// Create the real email and send it.

	log.Info("Preparing real email")
	realMail := constructEmail(sender, *emailSubject, htmlContent, filename, files)
	p = mail.NewPersonalization()
	for _, r := range to {
		p.AddTos(mail.NewEmail(r.Name, r.Address))
	}
	for _, r := range cc {
		p.AddCCs(mail.NewEmail(r.Name, r.Address))
	}
	for _, r := range bcc {
		p.AddBCCs(mail.NewEmail(r.Name, r.Address))
	}
	realMail.AddPersonalizations(p)

	log.Info("Sending to SendGrid")
	if _, err := sg.Send(realMail); err != nil {
		log.WithError(err).Fatal("Failed to send email")
	}

	log.Info("Email SENT!!")
}

func constructEmail(sender *netmail.Address, subject string, html string, filename string, files map[string][]byte) *mail.SGMailV3 {
	m := mail.NewV3Mail()
	from := mail.NewEmail(sender.Name, sender.Address)
	m.SetFrom(from)
	m.Subject = subject
	c := mail.NewContent("text/html", html)
	m.AddContent(c)
	log.WithField("count", len(files)-1).Info("Adding attachments")
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
	return m
}

func parseAddressList(str string) ([]*netmail.Address, error) {
	if str == "" {
		return []*netmail.Address{}, nil
	}
	return netmail.ParseAddressList(str)
}

func confirm(s string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [type 'yes' to confirm]: ", s)
	response, err := reader.ReadString('\n')
	if err != nil {
		log.WithError(err).Fatal("Failed to read input")
	}
	response = strings.ToLower(strings.TrimSpace(response))
	if response == "yes" {
		return true
	}
	return false
}
