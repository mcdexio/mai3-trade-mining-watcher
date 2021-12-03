package main

import (
	"encoding/csv"
	"fmt"
	"github.com/shopspring/decimal"
	"os"
	"strings"
)

var readScoreCsvPath = "/Users/champ/Downloads/2021.11.30epoch3-sql1.csv"
var readFeeCsvPath = "/Users/champ/Downloads/2021.11.30epoch3-sql2.csv"

var writeCsvPathBsc = "/Users/champ/Downloads/2021.11.30epoch3BscProportion.csv"
var writeCsvPathArb = "/Users/champ/Downloads/2021.11.30epoch3ArbProportion.csv"

func main() {
	decimal.DivisionPrecision = 21
	mcbAmount, err := decimal.NewFromString("54000000000000000000000")
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
		if score.Equal(decimal.Zero) {
			continue
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
	var disperseARB decimal.Decimal
	var disperseBsc decimal.Decimal
	for trader, chains := range traderMap {
		score := traderScoreMap[trader]
		proportion := score.Div(totalSum)
		mcbAmountOne := score.Mul(mcbAmount).Div(totalSum)
		if len(chains) == 1 {
			var w *csv.Writer
			mcbOne := mcbAmountOne.RoundDown(0)
			x := []string{trader}
			x = append(x, score.String())
			x = append(x, proportion.String())
			x = append(x, mcbOne.String())
			disperse = disperse.Add(mcbOne)
			if _, match := chains["0"]; match {
				w = wBsc
				disperseBsc = disperseBsc.Add(mcbOne)
			} else {
				w = wArb
				disperseARB = disperseARB.Add(mcbOne)
			}
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
			mcbOneBsc := mcbAmountOne.Mul(bscProportion).RoundDown(0)
			xBsc = append(xBsc, score.Mul(bscProportion).String())
			xBsc = append(xBsc, proportion.Mul(bscProportion).String())
			xBsc = append(xBsc, mcbOneBsc.String())
			disperse = disperse.Add(mcbOneBsc)
			if err = wBsc.Write(xBsc); err != nil {
				panic(err)
			}
			disperseBsc = disperseBsc.Add(mcbOneBsc)

			mcbOneArb := mcbAmountOne.Mul(arbProportion).RoundDown(0)
			xArb := []string{trader}
			xArb = append(xArb, score.Mul(arbProportion).String())
			xArb = append(xArb, proportion.Mul(arbProportion).String())
			xArb = append(xArb, mcbOneArb.String())
			disperse = disperse.Add(mcbOneArb)
			if err = wArb.Write(xArb); err != nil {
				panic(err)
			}
			disperseARB = disperseARB.Add(mcbOneArb)
		}
	}
	fmt.Printf("disperse %s, bsc %s, arb %s\n", disperse.String(), disperseBsc.String(), disperseARB)
}
