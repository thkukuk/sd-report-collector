// Command sd-report-certs manages the self-signed CA, server certificate,
// and client certificates used for sd-report-collector's mTLS, storing them
// below /etc/sd-report-collector.
package main

import (
	"flag"
	"fmt"
	"os"

	"sd-report-collector/internal/certgen"
)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage:
  sd-report-certs init [--force]
      Generate the CA and server certificate/key, and a config drop-in
      pointing sd-report-collector at them.

  sd-report-certs client <name> [--force]
      Generate a certificate/key for client <name>, signed by the CA.

Certificates are stored below %s.
`, certgen.DefaultBaseDir)
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	switch os.Args[1] {
	case "init":
		fs := flag.NewFlagSet("init", flag.ExitOnError)
		force := fs.Bool("force", false, "regenerate an existing CA/server certificate")
		fs.Parse(os.Args[2:])
		if fs.NArg() != 0 {
			usage()
		}
		if err := certgen.Init(certgen.DefaultBaseDir, *force); err != nil {
			fmt.Fprintln(os.Stderr, "sd-report-certs:", err)
			os.Exit(1)
		}

	case "client":
		fs := flag.NewFlagSet("client", flag.ExitOnError)
		force := fs.Bool("force", false, "overwrite an existing client certificate")
		fs.Parse(os.Args[2:])
		if fs.NArg() != 1 {
			usage()
		}
		if err := certgen.Client(certgen.DefaultBaseDir, fs.Arg(0), *force); err != nil {
			fmt.Fprintln(os.Stderr, "sd-report-certs:", err)
			os.Exit(1)
		}

	default:
		usage()
	}
}
