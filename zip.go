package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/apex/log"
)

func readZip(r io.Reader) (*zip.Reader, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve file: %s", err)
	}
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %s", err)
	}
	return z, nil
}

func parseZipFile(r *zip.Reader) (string, map[string][]byte) {
	fileMap := map[string][]byte{}
	htmlFile := ""
	for i, f := range r.File {
		if strings.HasSuffix(f.Name, ".html") {
			if htmlFile != "" {
				log.WithFields(log.Fields{
					"first":  htmlFile,
					"second": f.Name,
				}).Fatal("Multiple HTML files in export, not supported")
			}
			htmlFile = f.Name
		}
		rc, err := f.Open()
		if err != nil {
			log.WithError(err).WithField("filename", f.Name).Fatal("Failed to open zipped file")
		}
		b, err := ioutil.ReadAll(rc)
		if err != nil {
			log.WithError(err).WithField("filename", f.Name).Fatal("Failed to read zipped file")
		}
		fileMap[f.Name] = b
		log.WithField("filename", f.Name).WithField("size", len(b)).Infof("File %d:", i)
		rc.Close()
	}
	return htmlFile, fileMap
}
