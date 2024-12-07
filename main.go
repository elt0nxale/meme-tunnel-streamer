package main

import (
	"embed"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/rs/cors"
	"github.com/urfave/cli/v2"
	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"

	"meme-fetcher/internal/server"
)

//go:embed web/*
var content embed.FS

func main() {
	app := &cli.App{
		Name:  "meme-sse-debugger",
		Usage: "Server-Sent Events Meme Debugger with Ngrok Tunneling",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "port",
				Value: 8080,
				Usage: "Local server port",
			},
			&cli.BoolFlag{
				Name:  "tunnel",
				Usage: "Enable Ngrok tunneling",
			},
		},
		Action: func(ctx *cli.Context) error {
			// Seed random number generator
			rand.Seed(time.Now().UnixNano())

			// Create server
			srv := server.NewServer(content)

			// Setup routes
			mux := srv.SetupRoutes()

			// CORS middleware
			handler := cors.Default().Handler(mux)

			// Port configuration
			port := fmt.Sprintf(":%d", ctx.Int("port"))

			// Optional Ngrok tunneling
			if ctx.Bool("tunnel") {
				tun, err := ngrok.Listen(ctx.Context,
					config.HTTPEndpoint(),
					ngrok.WithAuthtokenFromEnv(),
				)
				if err != nil {
					return fmt.Errorf("ngrok listen failed: %v", err)
				}

				log.Printf("Tunnel available at: %s", tun.URL())
				return http.Serve(tun, handler)
			}

			// Standard local server
			log.Printf("Server starting on %s", port)
			return http.ListenAndServe(port, handler)
		},
	}

	// Run the CLI app
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
