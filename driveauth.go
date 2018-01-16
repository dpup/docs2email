package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"

	"github.com/apex/log"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
)

// From: https://developers.google.com/drive/v3/web/quickstart/go

// getClient uses a Context and Config to retrieve a Token
// then generate a Client. It returns the generated Client.
func getClient(ctx context.Context, config *oauth2.Config) *http.Client {
	cacheFile, err := tokenCacheFile()
	if err != nil {
		log.Fatalf("Unable to get path to cached credential file. %v", err)
	}
	tok, err := tokenFromFile(cacheFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(cacheFile, tok)
	}
	return config.Client(ctx, tok)
}

// getTokenFromWeb uses Config to request a Token.
// It returns the retrieved Token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	if err := openbrowser(authURL); err != nil {
		log.Warn("Go to the following link in your browser:")
		log.Warn(authURL)
		log.Warn("Then type the authorization code: ")
	} else {
		log.Warn("Follow the instructions in your browser then type the authorization code: ")
	}

	code, err := terminal.ReadPassword(0)
	if err != nil {
		log.WithError(err).Fatal("Unable to read authorization code")
	}

	tok, err := config.Exchange(oauth2.NoContext, string(code))
	if err != nil {
		log.WithError(err).Fatal("Unable to retrieve token from web")
	}
	return tok
}

// tokenCacheFile generates credential file path/filename.
// It returns the generated credential path/filename.
func tokenCacheFile() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	tokenCacheDir := filepath.Join(usr.HomeDir, ".credentials")
	os.MkdirAll(tokenCacheDir, 0700)
	return filepath.Join(tokenCacheDir, url.QueryEscape("docs2email.json")), err
}

// tokenFromFile retrieves a Token from a given file path.
// It returns the retrieved Token and any read error encountered.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	t := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(t)
	defer f.Close()
	return t, err
}

// saveToken uses a file path to create a file and store the
// token in it.
func saveToken(file string, token *oauth2.Token) {
	log.WithField("filename", file).Info("Saving credentials")
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.WithError(err).Fatal("Unable to cache oauth token")
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}
