package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/vanng822/go-premailer/premailer"
)

var (
	// Matches style attributes.
	reStyleAttr = regexp.MustCompile("style=\"[^\"]+\"")

	// GDocs sends a lot of inline styles. We strip all of them except those which
	// are used in place of styling such as <b> and <i>.
	allowedGDocStyles = map[string]bool{
		"font-style:italic": true,
		"font-weight:700":   true,
	}

	htmlHeader = `<html><head><style>` + inlineStyles + `</style></head><body><div class="body">`

	htmlFooter = `</div></body></html>`

	// Indicates an anchor that links to the comment block.
	reAnchor = regexp.MustCompile(`<sup><a href="#cmnt\d+" id="cmnt_ref\d+" rel="nofollow">\[\w+\]</a></sup>`)

	// Indicates the start of a comment block at the end of the doc.
	reComments = regexp.MustCompile(`<p><span>Comments</span></p><div><p>.*$`)

	// Matches spans with no attributes and no child elements.
	reEmptySpan = regexp.MustCompile(`<span\s*>([^<]*)</span>`)

	// GDocs often adds styling to headers via a span within the tag.
	reHeaderSpan = regexp.MustCompile(`(<h\d id="[^"]+">)<span[^>]+>([^<]*)</span>(</h\d>)`)
)

func cleanHTML(html string) (string, error) {
	// Clean up the HTML that Google sends us.
	sanitizer := bluemonday.UGCPolicy()
	sanitizer.AllowStyling()
	sanitizer.AllowAttrs("style").OnElements("span")
	html = sanitizer.Sanitize(html)

	// Google doesn't use markup for basic styling so remove all the junk but keep
	// the parts responsible for bold and italic.
	html = cleanStyles(html)

	// Google Docs includes comments in the export. We don't want to send them in
	// the email so strip out. We do this before passing to bluemonday so that
	// they are easier to identify through simple regexp matching.
	html = cleanComments(html)

	// After sanitization we end off with a lot of empty spans that amy cause
	// problems down the line.
	html = cleanSpans(html)

	// Headers often have extra styles applied via a nested <span> which will
	// override the styles added by premailer below. So jus strip 'em out.
	html = cleanHeaders(html)

	// Inline additional styles
	styler := premailer.NewPremailerFromString(htmlHeader+html+htmlFooter, premailer.NewOptions())
	styledContent, err := styler.Transform()
	if err != nil {
		return "", fmt.Errorf("Failed inlining styles: %s", err)
	}
	return styledContent, nil
}

func cleanStyles(html string) string {
	return reStyleAttr.ReplaceAllStringFunc(html, func(styles string) string {
		styles = strings.TrimPrefix(styles, "style=\"")
		styles = strings.TrimSuffix(styles, "\"")
		in := strings.Split(styles, ";")
		out := []string{}
		for _, s := range in {
			if ok := allowedGDocStyles[s]; ok {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return ""
		}
		return fmt.Sprintf("style=\"%s\"", strings.Join(out, ";"))
	})
}

func cleanComments(html string) string {
	html = reAnchor.ReplaceAllString(html, "")
	html = reComments.ReplaceAllString(html, "")
	return html
}

func cleanSpans(html string) string {
	return reEmptySpan.ReplaceAllStringFunc(html, func(span string) string {
		return reEmptySpan.FindStringSubmatch(span)[1]
	})
}

func cleanHeaders(html string) string {
	return reHeaderSpan.ReplaceAllStringFunc(html, func(header string) string {
		m := reHeaderSpan.FindStringSubmatch(header)
		return m[1] + m[2] + m[3]
	})
}
