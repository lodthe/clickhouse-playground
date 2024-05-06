package main

import (
	"clickhouse-playground/internal/testprocessor"
	"clickhouse-playground/internal/testprocessor/runs"
	"clickhouse-playground/pkg/playgroundclient"

	"flag"
	"fmt"
	"time"
)

type Action string

const (
	RunTest    Action = "run-test"
	ImportRuns Action = "import-runs"
)

func main() {
	var action string
	var needHelp bool
	var importRunsBefore string
	var importRunsAfter string
	var outputFile string

	flag.StringVar(&action, "action", "", "Action to process: run-test, import-runs")
	flag.BoolVar(&needHelp, "help", false, "Need to print help info for actions")
	flag.StringVar(&importRunsBefore, "before", "", "Right border for runs import from AWS. Format YYYY-MM-DD HH-MM-SS. For value \"2024-01-01 10:00:00\" script will load runs before 10:00:00 of 1st January 2024")
	flag.StringVar(&importRunsAfter, "after", "", "Left border for runs import from AWS. Format YYYY-MM-DD HH-MM-SS. For value \"2024-01-01 00:00:00\" script will load runs after 00:00:00 of 1st January 2024")
	flag.StringVar(&outputFile, "output", "imported_runs_data.yml", "Import runs output file path")
	flag.Parse()

	if needHelp && action == "" {
		fmt.Println("This is a script for testing playground and calculate requests elapsed time.\n" +
			"Specify action type by passing --action argument.\n\n" +
			"import-runs: import runs data from AWS for specified time period.\n" +
			"Use --action=import-runs --help for more information.\n\n" +
			"runs-test: runs test script, which takes runs data (database, version, query) and sends request to playground, saving elapsed time for each request and calculating percentiles.\n" +
			"Use --action=run-test --help for more information.")
		return
	}

	config, err := LoadConfig()
	if err != nil {
		fmt.Printf("config cannot be loaded: %s\n", err)
		return
	}

	switch Action(action) {
	case RunTest:
		if needHelp {
			fmt.Println("Runs test requests in playground and calculate elapsed time.\n" +
				"Supported modes: serial, serial-without-delays, parallel.\n\n" +
				"serial:\t\t\tprocess request from data file with delays between them (calculated by request time)\n" +
				"serial-without-delays:\tprocess request from data file without delays\n" +
				"parallel:\t\tprocess request in parallel running goroutines\n\n" +
				"You can specify output csv file path by setting \"output_path\" argument in config")
			return
		}
		runTest(config)
	case ImportRuns:
		if needHelp {
			fmt.Println("Imports runs data from DynamoDB with given time borders.\n\n" +
				"Arguments:\n" +
				"--before:\tsets right border for import. It sets as current time by default.\n" +
				"--after:\tsets left border for import. This is a required argument.\n\n" +
				"--before and --after arguments must be in \"YYYY-MM-DD HH-MM-SS\" format.")
			return
		}

		if importRunsAfter == "" {
			fmt.Println("Please, specify left time boarder for runs import by passing --after argument. Use --help for more information.")
			return
		}

		var runsAfter, runsBefore time.Time

		runsAfter, err = time.Parse(time.DateTime, importRunsAfter)
		if err != nil {
			fmt.Printf("invalid time format for \"after\" argument: %s\n", err)
			return
		}

		runsBefore = time.Now()
		if importRunsBefore != "" {
			runsBefore, err = time.Parse(time.DateTime, importRunsBefore)
			if err != nil {
				fmt.Printf("invalid time format for \"before\" argument: %s\n", err)
				return
			}
		}

		if runsBefore.Before(runsAfter) {
			fmt.Println("Right time border cannot be before left time border")
			return
		}

		err = runs.ImportRunsFromAWS(&runs.ImportRunsParams{
			AwsAccessKeyID:        config.AWS.AccessKeyID,
			AwsSecretAccessKey:    config.AWS.SecretAccessKey,
			AwsRegion:             config.AWS.Region,
			AwsQueryRunsTableName: config.AWS.QueryRunsTableName,
			RunsBefore:            runsBefore,
			RunsAfter:             runsAfter,
			OutputPath:            outputFile,
		})
		if err != nil {
			fmt.Printf("Faiiled to import runs: %s\n", err)
		}

	default:
		fmt.Println("Unknown action type. Supported: run-test, import-runs. Use --help for more information.")
	}
}

func runTest(config *Config) {
	playgroundClient := playgroundclient.New(&playgroundclient.Config{
		BaseURL: config.Playground.BaseURL,
	})

	testProcessor := testprocessor.New(&testprocessor.Config{
		Mode:         config.TestScript.Mode,
		RunsDataPath: config.TestScript.RunsDataPath,
		OutputPath:   *config.TestScript.OutputPath,
		DefaultQuery: config.TestScript.DefaultQuery,
		Percentiles:  config.TestScript.Percentiles,
	})

	err := testProcessor.Process(playgroundClient)
	if err != nil {
		fmt.Printf("Failed to process test script: %s\n", err)
	}
}
