package swagger

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/go-openapi/spec"
)

const (
	GCS_DOMAIN = "https://storage.googleapis.com"

	EXT_XLOGO_KEY = "x-logo"
	LOGO_FILENAME = "compass_logo.png"
	LOGO_ALTTEXT  = "Compass IoT logo"

	PUBLIC_DOCS_BUCKET   = "compass-public-docs"
	INTERNAL_DOCS_BUCKET = "compass-internal-docs"
)

// makeGcsUrl is a robust function which concatenates our GCS domain url,
// provided bucket name and nested file path
func makeGcsUrl(bucket string, args ...string) string {
	merged := []string{}
	merged = append(merged, bucket)
	merged = append(merged, args...)

	// Use `url` lib so we don't have to worry about slashes.
	// Also it only errors if the first argument does not have a scheme (i.e. http/https).
	// Since our hardcoded constant has it, I think we can safely ignore the error.
	str, _ := url.JoinPath(GCS_DOMAIN, merged...)
	return str
}

// getLabel generates a filename minus extension
func getLabel(file string) string {
	// Need to use `filepath` here as we're dealing with files
	return strings.TrimSuffix(file, filepath.Ext(file))
}

// getTemplateFile accepts a .proto file path and returns
// its template .html file path in the provided directory
func getTemplateFile(templateDir, protoFile string) string {
	// As convention, we're assuming that the template file has the same
	// filename as the proto file, minus the extension. Hence, we replace
	// the `proto` extension with `html`
	baseProto := filepath.Base(protoFile)
	baseHtml := strings.ReplaceAll(baseProto, "proto", "html")
	return filepath.Join(templateDir, baseHtml)
}

// parseTemplate reads a template file and injects it with data
func parseTemplate(templateFile string, data interface{}) (string, error) {
	// Load template file as string bytes. If it returns an error, we assume
	// that the file does not exist (even if that's not the case). When that
	// happens, we just return empty string and no error
	templateBytes, err := os.ReadFile(templateFile)
	if err != nil {
		return "", nil
	}
	templateString := string(templateBytes)

	// Parse template so the template engine knows the variables to be injected
	label := getLabel(templateFile)
	parsed, err := template.New(label).Parse(templateString)
	if err != nil {
		return "", err
	}

	// Execute template (inject values into the parsed template)
	// Since we may be doing string concatenation, use `strings.Builder`
	// to minimise memory copying: https://pkg.go.dev/strings#Builder
	var executed strings.Builder
	if err = parsed.Execute(&executed, data); err != nil {
		return "", err
	}

	// Return filled string
	return executed.String(), nil
}

// mapSdkFiles takes the comma-separated list of files in sw.sdkfiles,
// and builds a map with the key being SDK filenames + extension joined,
// and the value being their url on GCS
func (sw *Writer) mapSdkFiles() map[string]string {
	sdkfilesMap := make(map[string]string)
	label := getLabel(sw.filename)
	for _, f := range sw.sdkfiles {
		key := path.Base(f)                    // get base file name
		key = strings.ReplaceAll(key, ".", "") // remove dots
		key = strings.ReplaceAll(key, "_", "") // remove underscores
		key = strings.ReplaceAll(key, "-", "") // remove hyphens
		key = strings.TrimSpace(key)           // remove trailing space
		sdkfilesMap[key] = makeGcsUrl(PUBLIC_DOCS_BUCKET, label, sw.version, f)
	}
	return sdkfilesMap
}

// MakeDescription tries to find a template file its filename;
// i.e. given a file called admin.proto, it checks if admin.html exists. If true,
// it will parse and inject data to the template file and return it as string.
// Else, if the file does not exist, it will simply return an empty string
func (sw *Writer) MakeDescription() string {
	templateFile := getTemplateFile(sw.templateDir, sw.filename)
	description, err := parseTemplate(templateFile, sw.mapSdkFiles())
	if err != nil {
		return ""
	}
	return description
}

type Image struct {
	Url             string `json:"url,omitempty"`
	BackgroundColor string `json:"backgroundColor,omitempty"`
	AltText         string `json:"altText,omitempty"`
}

// MakeLogo creates a spec.Extensions object containing metadata to load
// logo in Swagger UI, such as its url and alt-text.
func (sw *Writer) MakeLogo() spec.Extensions {
	ext := make(spec.Extensions)
	xlogo := &Image{
		Url:     makeGcsUrl(PUBLIC_DOCS_BUCKET, LOGO_FILENAME),
		AltText: "Compass IoT logo",
	}
	ext.Add("x-logo", xlogo)
	return ext
}
