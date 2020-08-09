package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func handleCommandLine() {
	app := &cli.App{
		Name:  "jrpcd",
		Usage: "JSON RPC cache and router",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "listen",
				Aliases: []string{"l"},
				EnvVars: []string{"LISTEN"},
				Usage:   "Listen for RPC requests on the given `INTERFACE`",
				Value:   DefaultInterface,
			},
			&cli.StringFlag{
				Name:    "certfile",
				Aliases: []string{"C"},
				EnvVars: []string{"CERTFILE"},
				Usage:   "`FILE` containing an SSL certificate",
			},
			&cli.StringFlag{
				Name:    "keyfile",
				Aliases: []string{"K"},
				EnvVars: []string{"KEYFILE"},
				Usage:   "`FILE` containing the private key for the SSL certificate",
			},
			&cli.StringFlag{
				Name:    "logfile",
				EnvVars: []string{"LOGFILE"},
				Usage:   "Output log info to the given file",
			},
			&cli.StringFlag{
				Name:    "loglevel",
				EnvVars: []string{"LOGLEVEL"},
				Usage:   "Log `LEVEL`(error, warn, info, debug, trace)",
				Value:   DefaultLogLevel,
			},
			&cli.StringFlag{
				Name:    "cachedir",
				Aliases: []string{"d"},
				EnvVars: []string{"CACHEDIR"},
				Usage:   "Cache RPC results in the given `PATH`",
			},
			&cli.StringSliceFlag{
				Name:    "backend",
				Aliases: []string{"b"},
				EnvVars: []string{"BACKEND"},
				Usage:   "Add a JSON RPC backend `[name=]URL`",
			},
			&cli.StringSliceFlag{
				Name:    "route",
				Aliases: []string{"r"},
				EnvVars: []string{"ROUTE"},
				Usage:   "Add a route `BACKEND=RPCMETHOD`",
			},
		},
		Action:          startServer,
		HideHelpCommand: true,
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func startServer(ctx *cli.Context) error {
	c := NewConfig()
	if ctx.IsSet("listen") {
		c.Interface = ctx.String("listen")
	}
	if ctx.IsSet("certfile") {
		c.CertFile = ctx.String("certfile")
	}
	if ctx.IsSet("keyfile") {
		c.KeyFile = ctx.String("keyfile")
	}
	if ctx.IsSet("cachedir") {
		c.CacheDir = ctx.String("cachedir")
	}
	if ctx.IsSet("logfile") {
		c.LogFile = ctx.String("logfile")
	}
	if ctx.IsSet("loglevel") {
		c.LogLevel = ctx.String("loglevel")
	}

	switch strings.ToLower(c.LogLevel) {
	case "err", "error":
		c.LogLevel = "error"
	case "wrn", "warn", "warning":
		c.LogLevel = "warn"
	case "inf", "info":
		c.LogLevel = "info"
	case "dbg", "debug":
		c.LogLevel = "debug"
	case "trc", "trace":
		c.LogLevel = "trace"
	case "":
		c.LogLevel = "info"
	default:
		return fmt.Errorf("invalid log level: %s", c.LogLevel)
	}

	for _, s := range ctx.StringSlice("backend") {
		if s == "" {
			continue
		}

		b, err := parseBackend(s)
		if err != nil {
			return errors.Wrap(err, "backend: "+s)
		}
		if _, ok := c.Backends[b.Name]; ok {
			return errors.New("backend re-defined: " + b.Name)
		}
		c.Backends[b.Name] = b
	}
	if _, ok := c.Backends["default"]; !ok {
		return errors.New("no 'default' backend defined`")
	}

	rspecs := ctx.StringSlice("route")
	rspecs = append(rspecs, "default=match:*")
	for _, s := range rspecs {
		if s == "" {
			continue
		}

		r, err := parseRoute(s)
		if err != nil {
			return errors.Wrap(err, "route: "+s)
		}
		for _, name := range r.Backends {
			b := c.Backends[name]
			if b == nil {
				return errors.New("route: " + s + ": no such backend: " + name)
			}
			r.backends = append(r.backends, b)
		}
		c.Routes = append(c.Routes, r)
	}

	if err := NewCache(c).Start(); err != nil {
		return errors.Wrap(err, "Failed to start server")
	}
	return nil
}
