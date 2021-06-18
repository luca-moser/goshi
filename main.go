package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"time"

	"github.com/iotaledger/goshimmer/client"
	"github.com/iotaledger/goshimmer/client/wallet"
	"github.com/iotaledger/goshimmer/client/wallet/packages/address"
	"github.com/iotaledger/goshimmer/client/wallet/packages/seed"
	"github.com/iotaledger/goshimmer/client/wallet/packages/sendoptions"
	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/iotaledger/hive.go/bitmask"
)

var (
	program         = flag.Int("program", 0, "the program to run")
	faucetPoWTarget = flag.Int("faucetPoWTarget", 22, "the faucet pow target")
	programPara     = flag.Int("programPara", 4, "the program concurrent execution count")
	faucetReqSleep  = flag.Duration("faucetReqSleep", 0, "the duration to wait between faucet requests")
	pollingInterval = flag.Duration("pollingInterval", time.Second, "the polling interval")
	nodeURI         = flag.String("node", "https://api.goshimmer.lucamoser.io", "the node to use")
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	flag.Parse()
	log.Printf("running program %d", *program)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	switch *program {
	case 0:
		for i := 0; i < *programPara; i++ {
			go spamFaucetRequests()
		}
	case 1:
		for i := 0; i < *programPara; i++ {
			go spamMessages()
		}
	case 2:
		spamConflicts()
	}
	<-c
}

func spamConflicts() {
	var (
		api                     = client.NewGoShimmerAPI(*nodeURI)
		addrIndex        uint64 = 0
		walletSeed              = seed.NewSeed()
		fundingBlockTime        = 20 * time.Second
		conn                    = wallet.NewWebConnector(*nodeURI)
		wallet1                 = walletFromSeed(walletSeed, conn)
		wallet2                 = walletFromSeed(walletSeed, conn)
		targetWall              = createWallet()
	)

	addrBase58 := wallet1.Seed().Address(addrIndex).Base58()
	requestFunds(addrBase58, api, queryNodeID(api))
	blockCtx, blockCtxCancelF := context.WithTimeout(context.Background(), fundingBlockTime)
	defer blockCtxCancelF()
	sum, funded := blockUntilFunded(blockCtx, 1_000_000, addrBase58, api)
	if !funded {
		log.Printf("could not fund address %s in %v", addrBase58, fundingBlockTime)
		return
	}
	log.Printf("addr %s is funded with %d tokens", addrBase58, sum)

	targetAddrBase58 := targetWall.Seed().Address(addrIndex)

	must(wallet1.Refresh(true))
	must(wallet2.Refresh(true))

	var wg sync.WaitGroup
	wg.Add(2)
	go sendFunds(&wg, wallet1, targetAddrBase58, sum)
	go sendFunds(&wg, wallet2, targetAddrBase58, sum)
	wg.Wait()

	log.Println("conflicts sent")
}

func sendFunds(wg *sync.WaitGroup, wallet *wallet.Wallet, targetAddrBase58 address.Address, sum uint64) {
	defer wg.Done()
	_, err := wallet.SendFunds(sendoptions.Destination(targetAddrBase58, sum, ledgerstate.ColorIOTA),
		sendoptions.WaitForConfirmation(false))
	if err != nil {
		fmt.Printf(err.Error())
		return
	}
}

func queryNodeID(api *client.GoShimmerAPI) string {
	infoRes, err := api.Info()
	must(err)
	return infoRes.IdentityID
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
	wall := createWallet()

	api := client.NewGoShimmerAPI(*nodeURI)
	infoRes, err := api.Info()
	must(err)
	log.Printf("will pledge to %s", infoRes.IdentityID)

	for i := 0; ; i++ {
		addrBase58 := wall.Seed().Address(uint64(i)).Base58()
		requestFunds(addrBase58, api, infoRes.IdentityID)
		time.Sleep(*faucetReqSleep)
	}
}

func walletFromSeed(walletSeed *seed.Seed, conn wallet.Connector) *wallet.Wallet {
	return wallet.New(wallet.Import(
		walletSeed, 0, []bitmask.BitMask{}, wallet.NewAssetRegistry("nectar")),
		wallet.GenericConnector(conn),
	)
}

func requestFunds(addr string, api *client.GoShimmerAPI, pledgeID string) {
	faucetRes, err := api.SendFaucetRequest(addr, *faucetPoWTarget, pledgeID)
	if err != nil {
		log.Printf("error doing faucet request: %s", err)
		return
	}
	log.Printf("did faucet request: %s", faucetRes.ID)
}

func blockUntilFunded(ctx context.Context, minAmount uint64, addr string, api *client.GoShimmerAPI) (uint64, bool) {
	for {
		addrRes, err := api.GetAddressOutputs(addr)
		if err != nil {
			log.Printf("unable to check outputs on %s for funding", addr)
			return 0, false
		}
		var sum uint64
		for _, output := range addrRes.Outputs {
			ledgerOutput, err := output.ToLedgerstateOutput()
			if err != nil {
				log.Printf("unable to convert output to ledger output: %s", err)
				return 0, false
			}
			balance, ok := ledgerOutput.Balances().Get(ledgerstate.ColorIOTA)
			if !ok {
				continue
			}
			sum += balance
		}
		if sum >= minAmount {
			return sum, true
		}
		select {
		case <-time.After(*pollingInterval):
			continue
		case <-ctx.Done():
			return 0, false
		}
	}
}

func createWallet() *wallet.Wallet {
	connector := wallet.GenericConnector(wallet.NewWebConnector(*nodeURI))
	wall := wallet.New(connector)
	return wall
}
