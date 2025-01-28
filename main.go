package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type TestRow struct {
	parameterName string
	Description   string
	Comparator    string
	Value         float64
	LowerLimit    float64
	UpperLimit    float64
	SerialName    string
	Filename      string
}

const d2 = 2.353 // For one operator,5 parts, 6 runs, d2 = 2.353 https://andrewmilivojevich.com/d2-values-for-the-distribution-of-the-average-range/
const snreg = `_[a-zA-Z0-9]{11}_`

var gitHash string // Git hash, set during build or retrieved at runtime

func main() {
	// Check if the Git hash is set during build
	if gitHash == "" {
		// Attempt to retrieve the Git hash at runtime
		hash, err := getGitHash()
		if err != nil {
			log.Printf("Warning: Unable to retrieve Git hash at runtime: %v", err)
			gitHash = "unknown"
		} else {
			gitHash = hash
		}
	}

	fmt.Printf("Running with Git hash: %s\n", gitHash)

	// Proceed with the rest of the program
	csvFiles, err := getCSVFiles(".")
	if err != nil {
		log.Fatalf("Error retrieving CSV files: %v", err)
	}

	// Process files and write results
	groupedTests := make(map[string][]TestRow)
	for _, file := range csvFiles {
		serialName := extractSerialName(file)
		if serialName == "" {
			log.Printf("Could not extract serial name from file: %s", file)
			continue
		}

		tests, err := processCSV(file, serialName)
		if err != nil {
			log.Printf("Error processing file %s: %v", file, err)
			continue
		}

		// Group tests by unique parameter_name and description combination
		for _, test := range tests {
			if test.Comparator == "GELE" {
				key := fmt.Sprintf("%s|%s", test.parameterName, test.Description)
				groupedTests[key] = append(groupedTests[key], test)
			}
		}
	}

	// Write grouped results
	err = writeGroupedResults("grr_summary.csv", groupedTests)
	if err != nil {
		log.Fatalf("Error writing grouped results: %v", err)
	}
	fmt.Println("Grouped results written to grr_summary.csv")

	// Write raw data
	err = writeRawData("raw_data.csv", groupedTests)
	if err != nil {
		log.Fatalf("Error writing raw data: %v", err)
	}
	fmt.Println("Raw data written to raw_data.csv")
}

func getGitHash() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error retrieving git hash: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
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

func indexOf(headers []string, column string) int {
	for i, header := range headers {
		if header == column {
			return i
		}
	}
	return -1
}

func extractSerialName(fileName string) string {
	re := regexp.MustCompile(snreg)
	match := re.FindString(fileName)
	if match != "" {
		return strings.Trim(match, "_")
	}
	return ""
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
func writeGroupedResults(outputFile string, groupedTests map[string][]TestRow) error {
	// Get the Git hash
	gitHash, err := getGitHash()
	if err != nil {
		return fmt.Errorf("failed to retrieve git hash: %w", err)
	}

	// Temporary storage for sorted data
	type ResultRow struct {
		TaskDescription        string
		LowerLimit             float64
		UpperLimit             float64
		Repeatability          float64
		Reproducability        float64
		TotalGRR               float64
		GRRTolerancePercentage float64
	}
	var results []ResultRow

	// Group and calculate GR&R
	for key, tests := range groupedTests {
		parts := strings.Split(key, "|")
		if len(parts) != 2 {
			return fmt.Errorf("invalid key format: %s", key)
		}
		parameterName, description := parts[0], parts[1]

		// Group by serial name and calculate ranges
		serialRanges := make(map[string]float64)
		for _, test := range tests {
			serialRanges[test.SerialName] = 0
		}
		for serial := range serialRanges {
			var values []float64
			for _, test := range tests {
				if test.SerialName == serial {
					values = append(values, test.Value)
				}
			}
			if len(values) > 0 {
				maxValue := max(values)
				minValue := min(values)
				serialRanges[serial] = maxValue - minValue
			}
		}

		// Calculate total mean range and GRP
		var totalRangeSum float64
		for _, r := range serialRanges {
			totalRangeSum += r
		}
		totalMeanRange := totalRangeSum / float64(len(serialRanges))
		k2 := 1 / d2
		gr := totalMeanRange * k2

		// Calculate GR&R (based solely on repeatability for single actor/part)
		values := []float64{}
		for _, test := range tests {
			values = append(values, test.Value)
		}
		grp := (gr / (tests[0].UpperLimit - tests[0].LowerLimit)) * 100
		results = append(results, ResultRow{
			TaskDescription:        fmt.Sprintf("%s-%s", parameterName, description),
			LowerLimit:             tests[0].LowerLimit,
			UpperLimit:             tests[0].UpperLimit,
			Repeatability:          gr,
			Reproducability:        0,
			TotalGRR:               gr,
			GRRTolerancePercentage: grp,
		})
	}
	// Sort results by GRR percentage in descending order
	sort.Slice(results, func(i, j int) bool {
		return results[i].GRRTolerancePercentage > results[j].GRRTolerancePercentage
	})
	// Write results to CSV
	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("unable to create output file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write Git hash as a comment at the top
	_, err = file.WriteString(fmt.Sprintf("# Script Git Hash: %s\n", gitHash))
	if err != nil {
		return fmt.Errorf("error writing git hash: %w", err)
	}

	// Write header
	err = writer.Write([]string{"parameter_name-description", "lower_limit", "upper_limit", "Repeatability [units]", "Reproducability [units]", "TotalGRR [units]", "GRRTolerancePercentage [%]"})
	if err != nil {
		return fmt.Errorf("error writing header: %w", err)
	}

	// Write sorted data
	for _, row := range results {
		err = writer.Write([]string{
			row.TaskDescription,
			fmt.Sprintf("%.5f", row.LowerLimit),
			fmt.Sprintf("%.5f", row.UpperLimit),
			fmt.Sprintf("%.5f", row.Repeatability),
			fmt.Sprintf("%.5f", row.Reproducability),
			fmt.Sprintf("%.5f", row.TotalGRR),
			fmt.Sprintf("%.5f", row.GRRTolerancePercentage),
		})
		if err != nil {
			return fmt.Errorf("error writing row: %w", err)
		}
	}

	return nil
}

func writeRawData(outputFile string, groupedTests map[string][]TestRow) error {
	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("unable to create output file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	err = writer.Write([]string{"serial_name", "parameter_name", "description", "comparator", "value", "lower_limit", "upper_limit", "filename"})
	if err != nil {
		return fmt.Errorf("error writing header: %w", err)
	}

	// Write all raw data rows
	for _, tests := range groupedTests {
		for _, test := range tests {
			err = writer.Write([]string{
				test.SerialName,
				test.parameterName,
				test.Description,
				test.Comparator,
				fmt.Sprintf("%.2f", test.Value),
				fmt.Sprintf("%.2f", test.LowerLimit),
				fmt.Sprintf("%.2f", test.UpperLimit),
				test.Filename,
			})
			if err != nil {
				return fmt.Errorf("error writing row: %w", err)
			}
		}
	}

	return nil
}
func max(values []float64) float64 {
	if len(values) == 0 {
		log.Fatal("max: slice is empty")
	}
	maxVal := values[0]
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}

func min(values []float64) float64 {
	if len(values) == 0 {
		log.Fatal("min: slice is empty")
	}
	minVal := values[0]
	for _, v := range values {
		if v < minVal {
			minVal = v
		}
	}
	return minVal
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
	parameterNameIdx := indexOf(headers, "parameter_name")
	descriptionIdx := indexOf(headers, "description")
	comparatorIdx := indexOf(headers, "comparator")
	valueIdx := indexOf(headers, "value")
	lowerLimitIdx := indexOf(headers, "lower_limit")
	upperLimitIdx := indexOf(headers, "upper_limit")

	if parameterNameIdx == -1 || descriptionIdx == -1 || comparatorIdx == -1 || valueIdx == -1 || lowerLimitIdx == -1 || upperLimitIdx == -1 {
		return nil, fmt.Errorf("missing required columns in file: %s", filePath)
	}

	// Compile regex to remove unwanted characters
	re := regexp.MustCompile(`[0-9.\s\\/:"*?<>|]+`)

	// Parse rows
	var tests []TestRow
	for i, record := range records[1:] {
		if len(record) < len(headers) {
			log.Printf("Skipping row %d in %s: incomplete data", i+2, filePath)
			continue
		}

		// Clean the parameter_name and description fields
		parameterName := re.ReplaceAllString(record[parameterNameIdx], "")
		description := re.ReplaceAllString(record[descriptionIdx], "")

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
			parameterName: parameterName,
			Description:   description,
			Comparator:    record[comparatorIdx],
			Value:         value,
			LowerLimit:    lowerLimit,
			UpperLimit:    upperLimit,
			SerialName:    serialName,
			Filename:      filePath,
		})
	}

	return tests, nil
}
