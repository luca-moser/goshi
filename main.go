package main

import (
	"flag"
	"log"
	"strconv"

	"github.com/iotaledger/goshimmer/client"
	"github.com/iotaledger/goshimmer/client/wallet"
)

var (
	program = flag.Int("program", 0, "the program to run")
	nodeURI = flag.String("node", "https://api.goshimmer.lucamoser.io", "the node to use")
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	flag.Parse()
	switch *program {
	case 0:
		spamFaucetRequests()
	case 1:
		spamMessages()
	}
}

func spamMessages() {
	api := client.NewGoShimmerAPI(*nodeURI)

	for i := 0; ; i++ {
		_, err := api.Data([]byte(strconv.Itoa(i)))
		if err != nil {
			log.Printf("unable to send message: %s", err)
		}
		log.Printf("sent msg")
	}
}

func spamFaucetRequests() {
	connector := wallet.GenericConnector(wallet.NewWebConnector(*nodeURI))
	wall := wallet.New(connector)

	api := client.NewGoShimmerAPI(*nodeURI)
	infoRes, err := api.Info()
	must(err)

	log.Printf("will pledge to %s", infoRes.IdentityID)

	for i := 0; ; i++ {
		addr := wall.Seed().Address(uint64(i))
		faucetRes, err := api.SendFaucetRequest(addr.Base58(), 22, infoRes.IdentityID)
		if err != nil {
			log.Printf("error doing faucet request: %s", err)
			continue
		}
		log.Printf("did faucet request: %s", faucetRes.ID)
	}
}
