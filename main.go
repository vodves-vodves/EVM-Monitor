package main

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/lmittmann/w3"
	w3eth "github.com/lmittmann/w3/module/eth"
	"github.com/sourcegraph/conc"
	"log"
	"math/big"
	"reflect"
)

const (
	chatId = 111111111111

	ethAddr = "https://etherscan.io/address/"
	ethTx   = "https://etherscan.io/tx/"

	bscAddr = "https://bscscan.com/address/"
	bscTx   = "https://bscscan.com/tx/"
)

var (
	funcSymbol   = w3.MustNewFunc("symbol()", "string")
	funcDecimals = w3.MustNewFunc("decimals()", "uint8")
	funcTransfer = w3.MustNewFunc("transfer(address to, uint256 value)", "bool")
	//
	methodSigData [4]byte

	bot, _ = tgbotapi.NewBotAPI("token")
	wg     conc.WaitGroup
	//balance  big.Int
)

func main() {

	accounts := map[string]struct{}{
		"0x28C6c06298d514Db089934071355E5743bf21d60": {},
		"0x3c783c21a0383057D128bae431894a5C19F9Cf06": {},
	}
	methodSigData = [4]byte{169, 5, 156, 187}
	funcTransfer.Selector = methodSigData

	defer wg.Wait()
	wg.Go(func() {
		check("https://rpc.ankr.com/eth", accounts, "ETH", "ETH", ethAddr, ethTx)
	})
	wg.Go(func() {
		check("https://rpc.ankr.com/bsc", accounts, "BSC", "BNB", bscAddr, bscTx)
	})
}

func check(rcpUrl string, accounts map[string]struct{}, chain string, coin string, chainAddr string, chainTx string) {
	var (
		blockNumber, latestBlockNumber big.Int
		block                          types.Block

		symbol, from string
		decimals     uint8
		to           common.Address
		value        big.Int
	)
	client := w3.MustDial(rcpUrl)
	defer client.Close()
	for {
		if err := client.Call(
			w3eth.BlockNumber().Returns(&blockNumber),
		); err != nil {
			fmt.Printf("Failed to fetch 1: %v\n", err)
			continue
		}
		if latestBlockNumber.Uint64() >= blockNumber.Uint64() {
			continue
		}
		latestBlockNumber = blockNumber
		fmt.Println(blockNumber.Uint64())
		if err := client.Call(
			w3eth.BlockByNumber(&blockNumber).Returns(&block),
		); err != nil {
			fmt.Printf("Failed to fetch 2: %v\n", err)
			if err := client.Call(
				w3eth.BlockByNumber(&blockNumber).Returns(&block),
			); err != nil {
				fmt.Printf("Failed to fetch 3: %v\n", err)
				continue
			}
		}

		for _, tx := range block.Transactions() {

			tx := tx
			to := to
			value := value
			symbol := symbol
			decimals := decimals
			from := from
			go func() {

				if tx.To() == nil {
					return
				}
				from = getTransactionMessage(tx).From.Hex()
				if _, ok := accounts[tx.To().Hex()]; ok {
					//receive
					if len(tx.Data()) > 0 {
						if tr := reflect.DeepEqual(tx.Data()[:4], []byte{169, 5, 156, 187}); !tr {
							return
						}

						if err := funcTransfer.DecodeArgs(tx.Data(), &to, &value); err != nil {
							fmt.Printf("Failed to decode event log: %v\n", err)
							return
						}

						if err := client.Call(
							w3eth.CallFunc(funcSymbol, *tx.To()).Returns(&symbol),
							w3eth.CallFunc(funcDecimals, *tx.To()).Returns(&decimals),
						); err != nil {
							fmt.Printf("Call failed: %v\n", err)
							return
						}
						fmt.Println("receive", to, w3.FromWei(&value, decimals), symbol, tx.Hash())
						message := getMessage(chainAddr, chainTx, to.Hex(), from, &value, decimals, chain, "Received", symbol, "from", tx.Hash().Hex())
						sendMessage(message, chatId)
					} else {
						fmt.Println("receive", tx.To(), w3.FromWei(tx.Value(), 18), coin, tx.Hash())
						message := getMessage(chainAddr, chainTx, tx.To().Hex(), from, tx.Value(), 18, chain, "Received", coin, "from", tx.Hash().Hex())
						sendMessage(message, chatId)
					}
				}
				if _, ok := accounts[from]; ok {
					//sent
					if len(tx.Data()) > 0 {
						if tr := reflect.DeepEqual(tx.Data()[:4], []byte{169, 5, 156, 187}); !tr {
							return
						}

						if err := funcTransfer.DecodeArgs(tx.Data(), &to, &value); err != nil {
							fmt.Printf("Failed to decode event log: %v\n", err)
							return
						}
						if err := client.Call(
							w3eth.CallFunc(funcSymbol, *tx.To()).Returns(&symbol),
							w3eth.CallFunc(funcDecimals, *tx.To()).Returns(&decimals),
						); err != nil {
							fmt.Printf("Call failed: %v\n", err)
							return
						}

						fmt.Println("sent", from, w3.FromWei(&value, decimals), symbol, tx.Hash())
						message := getMessage(chainAddr, chainTx, from, to.Hex(), &value, decimals, chain, "Sent", symbol, "to", tx.Hash().Hex())
						sendMessage(message, chatId)

					} else {
						fmt.Println("sent", from, w3.FromWei(tx.Value(), 18), coin, tx.Hash())
						message := getMessage(chainAddr, chainTx, from, tx.To().Hex(), tx.Value(), 18, chain, "Sent", coin, "to", tx.Hash().Hex())
						sendMessage(message, chatId)
					}
				}
			}()
		}
	}

}

func getTransactionMessage(tx *types.Transaction) *core.Message {
	//msg, err := tx.AsMessage(types.LatestSignerForChainID(tx.ChainId()), nil)
	msg, err := core.TransactionToMessage(tx, types.LatestSignerForChainID(tx.ChainId()), nil)
	if err != nil {
		log.Fatal(err)
	}
	return msg
}

func sendMessage(message string, chatId int64) {
	msg := tgbotapi.NewMessage(chatId, message)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	bot.Send(msg)
}

func getMessage(scanAddr string, scanTx string, wallet string, address string, value *big.Int, decimals uint8, chain string, method string, symbol string, dist string, hash string) string {
	return fmt.Sprintf("Wallet <a href='%s%s'>%s</a> %s\n%s %v $%v %s <a href='%s%s'>%s</a>\n<a href='%s%s'>Tx hash</a>\n",
		scanAddr,
		wallet,
		fmt.Sprintf("%s-%s", wallet[:6], wallet[len(wallet)-4:]),
		chain,
		method,
		w3.FromWei(value, decimals),
		symbol,
		dist,
		scanAddr,
		address,
		fmt.Sprintf("%s-%s", address[:6], address[len(address)-4:]),
		scanTx,
		hash)
	//message := fmt.Sprintf("Wallet <a href='%s%s'>%s</a>\nSent %v $%v to <a href='%s%s'>%s</a>\n<a href='%s%s'>Tx hash</a>\n",
	//	ethAddr,
	//	from,
	//	fmt.Sprintf("%s-%s", from[:6], from[len(from)-4:]),
	//	w3.FromWei(&value, decimals),
	//	symbol,
	//	ethAddr,
	//	to.Hex(),
	//	fmt.Sprintf("%s-%s", to.Hex()[:6], to.Hex()[len(to.Hex())-4:]),
	//	ethTx,
	//	tx.Hash().Hex())
}
