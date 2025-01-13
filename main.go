package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type TestRow struct {
	TaskName    string
	Description string
	Comparator  string
	Value       float64
	LowerLimit  float64
	UpperLimit  float64
	SerialName  string
}

func main() {
	// Get all CSV files in the working directory
	csvFiles, err := getCSVFiles(".")
	if err != nil {
		log.Fatalf("Error retrieving CSV files: %v", err)
	}

	// Process each CSV file and aggregate data
	groupedTests := make(map[string][]TestRow)
	for _, file := range csvFiles {
		serialName := extractSerialName(file)
		if serialName == "" {
			log.Printf("Could not extract serial name from file: %s", file)
			continue
		}

		fmt.Printf("Processing file: %s\n", file)
		tests, err := processCSV(file, serialName)
		if err != nil {
			log.Printf("Error processing file %s: %v", file, err)
			continue
		}

		// Group tests by unique task_name and description combination
		for _, test := range tests {
			if test.Comparator == "GELE" {
				key := fmt.Sprintf("%s|%s", test.TaskName, test.Description)
				groupedTests[key] = append(groupedTests[key], test)
			}
		}
	}

	// Write grouped results to a new CSV file
	err = writeGroupedResults("grouped_results_single_actor.csv", groupedTests)
	if err != nil {
		log.Fatalf("Error writing grouped results: %v", err)
	}

	fmt.Println("Grouped results written to grouped_results_single_actor.csv")
}

func getCSVFiles(dir string) ([]string, error) {
	var csvFiles []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".csv") {
			csvFiles = append(csvFiles, path)
		}
		return nil
	})
	return csvFiles, err
}

func processCSV(filePath string, serialName string) ([]TestRow, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("unable to open file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("error reading CSV: %w", err)
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("CSV file must contain a header and at least one row of data")
	}

	// Get column indices
	headers := records[0]
	taskNameIdx := indexOf(headers, "task_name")
	descriptionIdx := indexOf(headers, "description")
	comparatorIdx := indexOf(headers, "comparator")
	valueIdx := indexOf(headers, "value")
	lowerLimitIdx := indexOf(headers, "lower_limit")
	upperLimitIdx := indexOf(headers, "upper_limit")

	if taskNameIdx == -1 || descriptionIdx == -1 || comparatorIdx == -1 || valueIdx == -1 || lowerLimitIdx == -1 || upperLimitIdx == -1 {
		return nil, fmt.Errorf("missing required columns in file: %s", filePath)
	}

	// Parse rows
	var tests []TestRow
	for i, record := range records[1:] {
		if len(record) < len(headers) {
			log.Printf("Skipping row %d in %s: incomplete data", i+2, filePath)
			continue
		}

		value, err := strconv.ParseFloat(record[valueIdx], 64)
		if err != nil {
			log.Printf("Skipping row %d in %s: invalid value", i+2, filePath)
			continue
		}

		lowerLimit, err := strconv.ParseFloat(record[lowerLimitIdx], 64)
		if err != nil {
			log.Printf("Skipping row %d in %s: invalid lower limit", i+2, filePath)
			continue
		}

		upperLimit, err := strconv.ParseFloat(record[upperLimitIdx], 64)
		if err != nil {
			log.Printf("Skipping row %d in %s: invalid upper limit", i+2, filePath)
			continue
		}

		tests = append(tests, TestRow{
			TaskName:    record[taskNameIdx],
			Description: record[descriptionIdx],
			Comparator:  record[comparatorIdx],
			Value:       value,
			LowerLimit:  lowerLimit,
			UpperLimit:  upperLimit,
			SerialName:  serialName,
		})
	}

	return tests, nil
}

func indexOf(headers []string, column string) int {
	for i, header := range headers {
		if header == column {
			return i
		}
	}
	return -1
}

func extractSerialName(fileName string) string {
	re := regexp.MustCompile(`_[A-Z0-9]{10,}_`)
	match := re.FindString(fileName)
	if match != "" {
		return strings.Trim(match, "_")
	}
	return ""
}

func writeGroupedResults(outputFile string, groupedTests map[string][]TestRow) error {
	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("unable to create output file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	err = writer.Write([]string{"task_name-description", "lower_limit", "upper_limit", "grr_percentage"})
	if err != nil {
		return fmt.Errorf("error writing header: %w", err)
	}

	// Write grouped data
	for key, tests := range groupedTests {
		parts := strings.Split(key, "|")
		if len(parts) != 2 {
			return fmt.Errorf("invalid key format: %s", key)
		}
		taskName, description := parts[0], parts[1]

		// Calculate GR&R (based solely on repeatability for single actor/part)
		values := []float64{}
		for _, test := range tests {
			values = append(values, test.Value)
		}
		repeatability := math.Sqrt(calculateVariance(values))
		grrPercentage := (repeatability / (tests[0].UpperLimit - tests[0].LowerLimit)) * 100

		err = writer.Write([]string{
			fmt.Sprintf("%s-%s", taskName, description),
			fmt.Sprintf("%.2f", tests[0].LowerLimit),
			fmt.Sprintf("%.2f", tests[0].UpperLimit),
			fmt.Sprintf("%.2f", grrPercentage),
		})
		if err != nil {
			return fmt.Errorf("error writing row: %w", err)
		}
	}

	return nil
}

func calculateVariance(values []float64) float64 {
	mean := calculateMean(values)
	var sum float64
	for _, v := range values {
		sum += (v - mean) * (v - mean)
	}
	return sum / float64(len(values))
}

func calculateMean(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
