package main

import (
	"encoding/csv"
	"fmt"
	"github.com/shopspring/decimal"
	"os"
	"strings"
)

var readCsvPath = "/Users/champ/Downloads/2021.11.17epoch2.csv"
var writeCsvPathBsc = "/Users/champ/Downloads/2021.11.17epoch2BscProportion.csv"
var writeCsvPathArb = "/Users/champ/Downloads/2021.11.17epoch2ArbProportion.csv"

func main() {
	decimal.DivisionPrecision = 21
	mcbAmount, err := decimal.NewFromString("63000000000000000000000")
	if err != nil {
		panic(err)
	}

	rFile, err := os.OpenFile(readCsvPath, os.O_RDONLY, 0777)
	if err != nil {
		panic(err)
	}
	r := csv.NewReader(rFile)
	defer rFile.Close()

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

	records, err := r.ReadAll()

	traderMap := make(map[string]map[string]decimal.Decimal, 0)
	traderScoreMap := make(map[string]decimal.Decimal, 0)
	totalSum := decimal.Zero
	totalChainCount := 0
	bscChainCount := 0
	arbChainCount := 0
	for _, record := range records[1:] {
		var score decimal.Decimal
		score, err = decimal.NewFromString(record[2])
		if err != nil {
			panic(err)
		}
		trader := strings.ToLower(record[0])
		chain := strings.ToLower(record[1])

		if chain == "total" {
			totalSum = totalSum.Add(score)
			traderScoreMap[trader] = score
			totalChainCount++
		} else if chain == "0" {
			bscChainCount++
		} else if chain == "1" {
			arbChainCount++
		}

		if trad, match := traderMap[trader]; match {
			trad[chain] = score
		} else {
			traderMap[trader] = make(map[string]decimal.Decimal)
			traderMap[trader][chain] = score
		}
	}
	fmt.Printf("totalSum=%s, totalChainCount=%d, bscChainCount=%d, arbChainCount=%d\n",
		totalSum.String(), totalChainCount, bscChainCount, arbChainCount,
	)

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
		if len(chains) == 2 || len(chains) == 1 {
			// default to arbitrum
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
		}

		if len(chains) == 3 {
			bscScore := chains["0"]
			arbScore := chains["1"]
			chainScore := bscScore.Add(arbScore)
			bscProportion := bscScore.Div(chainScore)
			arbProportion := arbScore.Div(chainScore)

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

		if !chains["total"].Equal(chains["0"].Add(chains["1"])) {
			fmt.Printf("trader %s score %+v is not equal\n", trader, chains)
		}
	}
	fmt.Printf("disperse %s\n", disperse.String())
}
