package main

import (
	"flag"

	"github.com/apex/log"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-bridget/twirp-swagger-gen/internal/swagger"
	"github.com/pkg/errors"
)

var _ = spew.Dump

func parse(hostname, filename, output string) error {
	if filename == output {
		return errors.New("output file must be different than input file")
	}

	writer := swagger.NewWriter(filename, hostname)
	if err := writer.WalkFile(); err != nil {
		return err
	}
	return writer.Save(output)
}

func main() {
	var (
		in   string
		out  string
		host string
	)
	flag.StringVar(&in, "in", "", "Input source .proto file")
	flag.StringVar(&out, "out", "", "Output swagger.json file")
	flag.StringVar(&host, "host", "api.example.com", "API host name")
	flag.Parse()

	if in == "" {
		log.Fatalf("Missing parameter: -in [input.proto]")
	}
	if out == "" {
		log.Fatalf("Missing parameter: -out [output.proto]")
	}
	if host == "" {
		log.Fatalf("Missing parameter: -host [api.example.com]")
	}

	if err := parse(host, in, out); err != nil {
		log.WithError(err).Fatal("exit with error")
	}
}
