package main

import (
	"encoding/csv"
	"fmt"
	"github.com/shopspring/decimal"
	"os"
	"strings"
)

var readScoreCsvPath = "/Users/champ/Downloads/2021.11.17epoch2-totalScore.csv"
var readFeeCsvPath = "/Users/champ/Downloads/2021.11.17epoch2-fee.csv"

var writeCsvPathBsc = "/Users/champ/Downloads/2021.11.17epoch2BscProportion.csv"
var writeCsvPathArb = "/Users/champ/Downloads/2021.11.17epoch2ArbProportion.csv"

func main() {
	decimal.DivisionPrecision = 21
	mcbAmount, err := decimal.NewFromString("63000000000000000000000")
	if err != nil {
		panic(err)
	}

	rScoreFile, err := os.OpenFile(readScoreCsvPath, os.O_RDONLY, 0777)
	if err != nil {
		panic(err)
	}
	rScore := csv.NewReader(rScoreFile)
	defer rScoreFile.Close()
	recordsScores, err := rScore.ReadAll()

	rFeeFile, err := os.OpenFile(readFeeCsvPath, os.O_RDONLY, 0777)
	if err != nil {
		panic(err)
	}
	rFee := csv.NewReader(rFeeFile)
	defer rFeeFile.Close()
	recordsFees, err := rFee.ReadAll()

	wBscFile, err := os.Create(writeCsvPathBsc)
	if err != nil {
		panic(err)
	}
	wBsc := csv.NewWriter(wBscFile)
	defer wBscFile.Close()
	defer wBsc.Flush()

	wArbFile, err := os.Create(writeCsvPathArb)
	if err != nil {
		panic(err)
	}
	wArb := csv.NewWriter(wArbFile)
	defer wArbFile.Close()
	defer wArb.Flush()

	totalSum := decimal.Zero
	traderScoreMap := make(map[string]decimal.Decimal, 0)
	allTradeScoreCount := 0
	for _, record := range recordsScores[1:] {
		var score decimal.Decimal
		score, err = decimal.NewFromString(record[1])
		if err != nil {
			panic(err)
		}
		totalSum = totalSum.Add(score)
		trader := strings.ToLower(record[0])
		traderScoreMap[trader] = score
		allTradeScoreCount++
	}
	fmt.Printf("totalSum=%s, allTradeScoreCount=%d\n", totalSum.String(), allTradeScoreCount)

	traderMap := make(map[string]map[string]decimal.Decimal, 0)
	bscChainCount := 0
	arbChainCount := 0
	for _, record := range recordsFees[1:] {
		trader := strings.ToLower(record[0])
		_, match := traderScoreMap[trader]
		if !match {
			// skip for total score == 0
			continue
		}

		chain := strings.ToLower(record[1])
		var fee decimal.Decimal
		fee, err = decimal.NewFromString(record[2])
		if err != nil {
			panic(err)
		}
		if chain == "0" {
			bscChainCount++
		} else if chain == "1" {
			arbChainCount++
		}
		if trade, match := traderMap[trader]; match {
			trade[chain] = fee
		} else {
			traderMap[trader] = make(map[string]decimal.Decimal)
			traderMap[trader][chain] = fee
		}
	}
	fmt.Printf("bscChainCount %d, arbChainCount %d\n", bscChainCount, arbChainCount)

	if err = wBsc.Write([]string{"trader", "score", "proportion", "MCB"}); err != nil {
		panic(err)
	}
	if err = wArb.Write([]string{"trader", "score", "proportion", "MCB"}); err != nil {
		panic(err)
	}

	var disperse decimal.Decimal
	for trader, chains := range traderMap {
		score := traderScoreMap[trader]
		proportion := score.Div(totalSum)
		mcbAmountOne := score.Mul(mcbAmount).Div(totalSum)
		if len(chains) == 1 {
			var w *csv.Writer
			if _, match := chains["0"]; match {
				w = wBsc
			} else {
				w = wArb
			}
			x := []string{trader}
			x = append(x, score.String())
			x = append(x, proportion.String())
			x = append(x, mcbAmountOne.RoundDown(0).String())
			disperse = disperse.Add(mcbAmountOne.RoundDown(0))
			if err = w.Write(x); err != nil {
				panic(err)
			}
			continue
		}

		if len(chains) == 2 {
			bscFee := chains["0"]
			arbFee := chains["1"]
			chainFee := bscFee.Add(arbFee)
			bscProportion := bscFee.Div(chainFee)
			arbProportion := arbFee.Div(chainFee)

			xBsc := []string{trader}
			xBsc = append(xBsc, score.Mul(bscProportion).String())
			xBsc = append(xBsc, proportion.Mul(bscProportion).String())
			xBsc = append(xBsc, mcbAmountOne.Mul(bscProportion).RoundDown(0).String())
			disperse = disperse.Add(mcbAmountOne.Mul(bscProportion).RoundDown(0))
			if err = wBsc.Write(xBsc); err != nil {
				panic(err)
			}

			xArb := []string{trader}
			xArb = append(xArb, score.Mul(arbProportion).String())
			xArb = append(xArb, proportion.Mul(arbProportion).String())
			xArb = append(xArb, mcbAmountOne.Mul(arbProportion).RoundDown(0).String())
			disperse = disperse.Add(mcbAmountOne.Mul(arbProportion).RoundDown(0))
			if err = wArb.Write(xArb); err != nil {
				panic(err)
			}
		}
	}
	fmt.Printf("disperse %s\n", disperse.String())
}
