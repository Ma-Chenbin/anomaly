// Copyright 2017 The Anomaly Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"

	"github.com/pointlander/anomaly"
)

const (
	// VectorSize is the size of the JSON document vector
	VectorSize = 1024
	// Samples is the number of JSON documents to generate per trial
	Samples = 1000
	// Trials is the number of trials
	Trials = 100
	// Parallelization is how many trials to perform in parallel
	Parallelization = 10
	// Cutoff is the number of initial samples to ignore
	Cutoff = 100
)

// Tests are basic tests for anomaly detection
var Tests = []string{`{
 "alfa": [
  {"alfa": "1"},
	{"bravo": "2"}
 ],
 "bravo": [
  {"alfa": "3"},
	{"bravo": "4"}
 ]
}`, `{
 "a": [
  {"a": "aa"},
	{"b": "bb"}
 ],
 "b": [
  {"a": "aa"},
	{"b": "bb"}
 ]
}`}

func tanh32(x float32) float32 {
	a, b := math.Exp(float64(x)), math.Exp(-float64(x))
	return float32((a - b) / (a + b))
}

func dtanh32(x float32) float32 {
	return 1 - x*x
}

// TestResult is a test result
type TestResult struct {
	Surprise    float64
	Uncertainty float64
	Raw         float64
}

// TestResults are the test results from Anomaly
type TestResults struct {
	Name            string
	Seed            int
	Surprise        plotter.Values
	Uncertainty     plotter.Values
	Average, STDDEV float64
	Results         []TestResult
}

func statistics(values plotter.Values) (average, stddev float64) {
	sum, sumSquared, length := 0.0, 0.0, float64(len(values))
	for _, v := range values {
		value := float64(v)
		sum += value
		sumSquared += value * value
	}
	average = sum / length
	stddev = math.Sqrt(sumSquared/length - average*average)
	return
}

// Anomaly tests the anomaly detection algorithms
func Anomaly(seed int, factory anomaly.NetworkFactory, name string) *TestResults {
	rndGenerator := rand.New(rand.NewSource(int64(seed)))
	rndNetwork := rand.New(rand.NewSource(int64(seed)))
	vectorizer := anomaly.NewVectorizer(VectorSize, true, anomaly.NewLFSR32Source)
	network := factory(rndNetwork, vectorizer)

	surprise, uncertainty := make(plotter.Values, Samples), make(plotter.Values, Samples)
	hasUncertainty := false
	for i := 0; i < Samples; i++ {
		object := anomaly.GenerateRandomJSON(rndGenerator)
		input, err := json.Marshal(object)
		if err != nil {
			panic(err)
		}
		s, u := network.Train(input)
		if u > 0 {
			hasUncertainty = true
		}
		surprise[i], uncertainty[i] = float64(s), float64(u)
	}
	surprise = surprise[Cutoff:]

	average, stddev := statistics(surprise)

	results := make([]TestResult, len(Tests))
	for i, test := range Tests {
		var object map[string]interface{}
		err := json.Unmarshal([]byte(test), &object)
		if err != nil {
			panic(err)
		}
		input, err := json.Marshal(object)
		if err != nil {
			panic(err)
		}
		s, u := network.Train([]byte(input))
		results[i].Raw = float64(s)
		results[i].Surprise = math.Abs((float64(s) - average) / stddev)
		results[i].Uncertainty = float64(u)
	}

	testResults := &TestResults{
		Name:     name,
		Seed:     seed,
		Surprise: surprise,
		Average:  average,
		STDDEV:   stddev,
		Results:  results,
	}
	if hasUncertainty {
		testResults.Uncertainty = uncertainty
	}
	return testResults
}

// IsCorrect determines if a result is IsCorrect
func (t *TestResults) IsCorrect() bool {
	return t.Results[0].Surprise > t.Results[1].Surprise
}

// Print prints test results
func (t *TestResults) Print() {
	results := t.Results
	if results[0].Uncertainty != 0 && results[1].Uncertainty != 0 {
		fmt.Printf("%v %v %.6f (%.6f+-%.6f) %.6f (%.6f+-%.6f)\n", t.Seed, t.Name,
			results[0].Surprise, results[0].Raw, results[0].Uncertainty,
			results[1].Surprise, results[1].Raw, results[1].Uncertainty)
		return
	}
	fmt.Printf("%v %v %.6f (%.6f) %.6f (%.6f)\n", t.Seed, t.Name,
		results[0].Surprise, results[0].Raw,
		results[1].Surprise, results[1].Raw)

}

var full = flag.Bool("full", false, "run full bench")

func main() {
	flag.Parse()

	graph := 1

	histogram := func(title, name string, values *TestResults) {
		p, err := plot.New()
		if err != nil {
			panic(err)
		}
		p.Title.Text = title

		h, err := plotter.NewHist(values.Surprise, 20)
		if err != nil {
			panic(err)
		}
		p.Add(h)

		err = p.Save(8*vg.Inch, 8*vg.Inch, fmt.Sprintf("graph_%v_%v", graph, name))
		if err != nil {
			panic(err)
		}

		graph++
	}

	scatterPlot := func(xTitle, yTitle, name string, xx, yy *TestResults) {
		xys := make(plotter.XYs, len(yy.Surprise))
		if xx == nil {
			for i, v := range yy.Surprise {
				xys[i].X = float64(i)
				xys[i].Y = v
			}
		} else {
			for i, v := range yy.Surprise {
				xys[i].X = xx.Surprise[i]
				xys[i].Y = v
			}
		}

		x, y, x2, y2, xy, n := 0.0, 0.0, 0.0, 0.0, 0.0, float64(len(xys))
		for i := range xys {
			x += xys[i].X
			y += xys[i].Y
			x2 += xys[i].X * xys[i].X
			y2 += xys[i].Y * xys[i].Y
			xy += xys[i].X * xys[i].Y
		}
		corr := (n*xy - x*y) / (math.Sqrt(n*x2-x*x) * math.Sqrt(n*y2-y*y))

		p, err := plot.New()
		if err != nil {
			panic(err)
		}

		p.Title.Text = fmt.Sprintf("%v vs %v corr=%v", yTitle, xTitle, corr)
		p.X.Label.Text = xTitle
		p.Y.Label.Text = yTitle

		s, err := plotter.NewScatter(xys)
		if err != nil {
			panic(err)
		}
		p.Add(s)

		if uncertainty := yy.Uncertainty; uncertainty != nil {
			errors := make(plotter.YErrors, len(uncertainty))
			for k, v := range uncertainty {
				errors[k].High = v
				errors[k].Low = v
			}
			y := &struct {
				plotter.XYs
				plotter.YErrors
			}{
				XYs:     xys,
				YErrors: errors,
			}
			bar, err1 := plotter.NewYErrorBars(y)
			if err1 != nil {
				panic(err1)
			}
			err1 = plotutil.AddErrorBars(p, bar)
			if err1 != nil {
				panic(err1)
			}
		}

		err = p.Save(8*vg.Inch, 8*vg.Inch, fmt.Sprintf("graph_%v_%v", graph, name))
		if err != nil {
			panic(err)
		}

		graph++
	}

	averageSimilarity := Anomaly(1, anomaly.NewAverageSimilarity, "average similarity")
	histogram("Average Similarity Distribution", "average_similarity_distribution.png", averageSimilarity)
	scatterPlot("Time", "Average Similarity", "average_similarity.png", nil, averageSimilarity)
	averageSimilarity.Print()

	neuron := Anomaly(1, anomaly.NewNeuron, "neuron")
	histogram("Neuron Distribution", "neuron_distribution.png", neuron)
	scatterPlot("Time", "Neuron", "neuron.png", nil, neuron)
	scatterPlot("Average Similarity", "Neuron", "neuron_vs_average_similarity.png",
		averageSimilarity, neuron)
	neuron.Print()

	autoencoderError := Anomaly(1, anomaly.NewAutoencoder, "autoencoder")
	histogram("Autoencoder Error Distribution", "autoencoder_error_distribution.png", autoencoderError)
	scatterPlot("Time", "Autoencoder Error", "autoencoder_error.png", nil, autoencoderError)
	scatterPlot("Average Similarity", "Autoencoder Error", "autoencoder_error_vs_average_similarity.png",
		averageSimilarity, autoencoderError)
	autoencoderError.Print()

	lstmError := Anomaly(1, anomaly.NewLSTM, "lstm")
	histogram("LSTM Distribution", "lstm_distribution.png", lstmError)
	scatterPlot("Time", "LSTM", "lstm.png", nil, lstmError)
	scatterPlot("Average Similarity", "LSTM", "lstm_vs_average_similarity.png",
		averageSimilarity, lstmError)
	lstmError.Print()

	gruError := Anomaly(1, anomaly.NewGRU, "gru")
	histogram("GRU Distribution", "gru_distribution.png", gruError)
	scatterPlot("Time", "GRU", "gru.png", nil, gruError)
	scatterPlot("GRU", "LSTM", "lstm_vs_gru.png", gruError, lstmError)
	gruError.Print()

	complexityError := Anomaly(1, anomaly.NewComplexity, "complexity")
	histogram("Complexity Distribution", "complexity_distribution.png", complexityError)
	scatterPlot("Time", "Complexity", "complexity.png", nil, complexityError)
	complexityError.Print()

	metaError := Anomaly(1, anomaly.NewMeta, "meta")
	scatterPlot("Time", "Meta", "meta.png", nil, metaError)
	metaError.Print()

	if !*full {
		return
	}

	test := func(factory anomaly.NetworkFactory, name string) int {
		count, total, results, j := 0, 0, make(chan *TestResults, Parallelization), 1
		process := func() {
			result := <-results
			result.Print()
			if result.IsCorrect() {
				count++
			}
			total++
		}
		for i := 0; i < Parallelization; i++ {
			go func(j int) {
				results <- Anomaly(j, factory, name)
			}(j)
			j++
		}
		for j <= Trials {
			process()
			go func(j int) {
				results <- Anomaly(j, factory, name)
			}(j)
			j++
		}
		for total < Trials {
			process()
		}
		return count
	}
	averageSimilarityCount := test(anomaly.NewAverageSimilarity, "average similarity")
	neuronCount := test(anomaly.NewNeuron, "neuron")
	autoencoderCount := test(anomaly.NewAutoencoder, "autoencoder")
	fmt.Printf("average similarity: %v / %v\n", averageSimilarityCount, Trials)
	fmt.Printf("neuron: %v / %v\n", neuronCount, Trials)
	fmt.Printf("autoencoder: %v / %v\n", autoencoderCount, Trials)
}
