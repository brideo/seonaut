package main

import (
	"flag"
	"log"
	_ "time/tzdata" // embed tz database so TZ env var works in Alpine without tzdata package

	"github.com/stjudewashere/seonaut/internal/api"
	"github.com/stjudewashere/seonaut/internal/services"
)

func main() {
	var configFile string

	flag.StringVar(&configFile, "c", "config", "Specify configuration file. Default is config.")
	flag.Parse()

	container := services.NewContainer(configFile)
	server := api.NewServer(container)

	log.Fatal(server.ListenAndServe())
}
