// Command sd-report-certs manages the self-signed CA, server certificate,
// and client certificates used for sd-report-collector's mTLS, storing them
// below /etc/sd-report-collector.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"sd-report-collector/internal/certgen"
)

// stringList collects repeated occurrences of a flag into a slice.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage:
  sd-report-certs init [--force] [--san=<name-or-ip>]...
      Generate the CA and server certificate/key, and a config drop-in
      pointing sd-report-collector at them. The server certificate's SAN
      list always includes the machine's hostname and "localhost"; pass
      --san (repeatable) to add further hostnames or IP addresses clients
      may use to reach the server, e.g. --san=192.168.3.16.

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
		var sans stringList
		fs.Var(&sans, "san", "additional hostname or IP address for the server certificate (repeatable)")
		fs.Parse(os.Args[2:])
		if fs.NArg() != 0 {
			usage()
		}
		if err := certgen.Init(certgen.DefaultBaseDir, sans, *force); err != nil {
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
