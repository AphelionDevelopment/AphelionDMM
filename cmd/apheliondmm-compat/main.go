package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"sdmm/internal/aphelion/collab/compat"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, output, errorOutput io.Writer) int {
	flags := flag.NewFlagSet("apheliondmm-compat", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	matrixPath := flags.String("matrix", "", "optional compatibility matrix JSON file")
	from := flags.String("from", "", "rolling upgrade source release")
	to := flags.String("to", "", "rolling upgrade target release")
	clientProtocol := flags.Uint("client-protocol", 0, "client protocol version")
	clientSchema := flags.Uint("client-schema", 0, "client snapshot schema version")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	matrix, err := loadMatrix(*matrixPath)
	if err != nil {
		_, _ = fmt.Fprintf(errorOutput, "load compatibility matrix: %v\n", err)
		return 1
	}
	rollingMode := *from != "" || *to != ""
	clientMode := *clientProtocol != 0 || *clientSchema != 0
	if rollingMode == clientMode {
		_, _ = fmt.Fprintln(errorOutput, "select exactly one mode: -from/-to or -client-protocol/-client-schema")
		return 2
	}
	if rollingMode {
		if *from == "" || *to == "" {
			_, _ = fmt.Fprintln(errorOutput, "both -from and -to are required")
			return 2
		}
		if err := matrix.ValidateRolling(*from, *to); err != nil {
			_, _ = fmt.Fprintf(errorOutput, "rolling upgrade rejected: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(output, "rolling upgrade %s -> %s is compatible\n", *from, *to)
		return 0
	}
	if *clientProtocol == 0 || *clientSchema == 0 || *clientProtocol > 65535 || *clientSchema > 65535 {
		_, _ = fmt.Fprintln(errorOutput, "valid -client-protocol and -client-schema values are required")
		return 2
	}
	negotiated, err := matrix.Negotiate(uint16(*clientProtocol), uint16(*clientSchema))
	if err != nil {
		_, _ = fmt.Fprintf(errorOutput, "client rejected: %v\n", err)
		return 1
	}
	if err := json.NewEncoder(output).Encode(negotiated); err != nil {
		_, _ = fmt.Fprintf(errorOutput, "write result: %v\n", err)
		return 1
	}
	return 0
}

func loadMatrix(path string) (compat.Matrix, error) {
	if path == "" {
		return compat.DefaultMatrix(), nil
	}
	file, err := os.Open(path)
	if err != nil {
		return compat.Matrix{}, err
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	var matrix compat.Matrix
	if err := decoder.Decode(&matrix); err != nil {
		return compat.Matrix{}, err
	}
	if err := matrix.Validate(); err != nil {
		return compat.Matrix{}, err
	}
	return matrix, nil
}
