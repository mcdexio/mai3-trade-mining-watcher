package main

import (
	"encoding/csv"
	"fmt"
	"github.com/shopspring/decimal"
	"os"
)

var readCsvPath = "/Users/champ/Downloads/2021.11.1epoch1.csv"
var writeCsvPath = "/Users/champ/Downloads/2021.11.1epoch1proportion.csv"
var mcbAmount = decimal.NewFromInt(72000)

func main() {
	rFile, err := os.OpenFile(readCsvPath, os.O_RDONLY, 0777)
	if err != nil {
		panic(err)
	}
	r := csv.NewReader(rFile)
	wFile, err := os.Create(writeCsvPath)
	if err != nil {
		panic(err)
	}
	w := csv.NewWriter(wFile)
	defer wFile.Close()
	defer rFile.Close()
	defer w.Flush()

	record, err := r.ReadAll()

	sum := decimal.Zero
	for _, r := range record[1:] {
		one, err := decimal.NewFromString(r[1])
		if err != nil {
			panic(err)
		}
		sum = sum.Add(one)
	}
	fmt.Println("sum", sum)

	if err := w.Write([]string{"trader", "score", "proportion", "MCB"}); err != nil {
		panic(err)
	}

	for _, r := range record[1:] {
		x := []string{r[0]} //traderID
		one, err := decimal.NewFromString(r[1])
		if err != nil {
			panic(err)
		}
		proportion := one.Div(sum)
		mcbAmountOne := mcbAmount.Mul(proportion)
		x = append(x, one.String())          // score
		x = append(x, proportion.String())   // proportion
		x = append(x, mcbAmountOne.String()) // mcb
		if err := w.Write(x); err != nil {
			panic(err)
		}
	}
}
