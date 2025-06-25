package main

import (
	"archive/zip"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
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

const d2 = 1.714 //2.558 // Use the d2_Lookup table to get the correct value
const snreg = `_[a-zA-Z0-9]{11}_`
const maxTestRuns = 3 // Maximum number of test runs to keep per serial number

var gitHash string // Git hash, set during build or retrieved at runtime

func main() {
	// Add force flag
	var forceFlag bool
	flag.BoolVar(&forceFlag, "f", false, "Force processing by ignoring limit consistency checks")
	flag.Parse()

	// Clean up existing output files and directories
	filesToRemove := []string{
		"data_filtered.csv",
		"grr_summary.csv",
	}
	for _, file := range filesToRemove {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: Could not remove existing file %s: %v", file, err)
		}
	}

	// Remove scatter_plots directory and its contents
	if err := os.RemoveAll("scatter_plots"); err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: Could not remove existing scatter_plots directory: %v", err)
	}

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

	// Read raw data from data_raw.csv
	file, err := os.Open("data_raw.csv")
	if err != nil {
		log.Fatalf("Error opening data_raw.csv: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		log.Fatalf("Error reading data_raw.csv: %v", err)
	}

	if len(records) < 2 {
		log.Fatal("data_raw.csv must contain a header and at least one row of data")
	}

	// Get column indices and validate headers
	headers := records[0]
	fmt.Printf("Found columns in data_raw.csv: %v\n", headers)

	// Define required columns and their possible variations
	columnMappings := map[string][]string{
		"serial_name":    {"serial_num", "serial_number", "serial"},
		"parameter_name": {"parameter_name", "param_name"},
		"description":    {"description", "desc"},
		"comparator":     {"comparator", "comp"},
		"value":          {"param_value_float", "value"},
		"lower_limit":    {"lower_limit", "lower"},
		"upper_limit":    {"upper_limit", "upper"},
		"filename":       {"test_id", "pon.test_id"},
		"result":         {"result", "pon.result"},
		"test_time":      {"test_start_time", "start_time"}, // Add test time column
		"tester_name":    {"tester_name"},
	}

	// Find column indices with flexible matching
	columnIndices := make(map[string]int)
	for requiredCol, possibleNames := range columnMappings {
		found := false
		for _, possibleName := range possibleNames {
			if idx := indexOf(headers, possibleName); idx != -1 {
				columnIndices[requiredCol] = idx
				found = true
				fmt.Printf("Found column '%s' as '%s'\n", requiredCol, possibleName)
				break
			}
		}
		if !found {
			log.Fatalf("Required column '%s' not found in data_raw.csv. Available columns: %v", requiredCol, headers)
		}
	}

	// First pass: collect all passing tests with their timestamps
	type TestRecord struct {
		record    []string
		timestamp string
		testID    string
	}
	passingTests := make(map[string][]TestRecord) // map[serial_num][]TestRecord

	// Process all rows to collect passing tests
	for i, record := range records[1:] {
		if len(record) < len(headers) {
			log.Printf("Skipping row %d: incomplete data (expected %d columns, got %d)", i+2, len(headers), len(record))
			continue
		}

		serialNum := strings.TrimSpace(record[columnIndices["serial_name"]])
		testResult := strings.TrimSpace(record[columnIndices["result"]])
		testTime := strings.TrimSpace(record[columnIndices["test_time"]])
		testID := strings.TrimSpace(record[columnIndices["filename"]])

		if serialNum != "" && (strings.ToLower(testResult) == "pass" || strings.ToLower(testResult) == "true") {
			passingTests[serialNum] = append(passingTests[serialNum], TestRecord{
				record:    record,
				timestamp: testTime,
				testID:    testID,
			})
		}
	}

	// Sort and filter passing tests
	filteredRecords := [][]string{headers}              // Start with headers
	selectedTestIDs := make(map[string]map[string]bool) // map[serial_num]map[test_id]bool

	// First, select the test IDs to keep
	for serialNum, tests := range passingTests {
		// Sort by timestamp in descending order
		sort.Slice(tests, func(i, j int) bool {
			return tests[i].timestamp > tests[j].timestamp
		})

		// Initialize map for this serial number
		selectedTestIDs[serialNum] = make(map[string]bool)

		// Select the latest unique test IDs
		uniqueTestIDs := make(map[string]bool)
		for _, test := range tests {
			if !uniqueTestIDs[test.testID] {
				uniqueTestIDs[test.testID] = true
				selectedTestIDs[serialNum][test.testID] = true
				if len(uniqueTestIDs) >= maxTestRuns {
					break
				}
			}
		}
	}

	// Now collect all rows for the selected test IDs
	for serialNum, tests := range passingTests {
		for _, test := range tests {
			if selectedTestIDs[serialNum][test.testID] {
				filteredRecords = append(filteredRecords, test.record)
			}
		}
	}

	// Write filtered data to CSV
	filteredFile, err := os.Create("data_filtered.csv")
	if err != nil {
		log.Fatalf("Error creating filtered data file: %v", err)
	}
	defer filteredFile.Close()

	writer := csv.NewWriter(filteredFile)
	defer writer.Flush()

	if err := writer.WriteAll(filteredRecords); err != nil {
		log.Fatalf("Error writing filtered data: %v", err)
	}

	fmt.Printf("\nFiltered data summary:\n")
	fmt.Printf("Total records in filtered data: %d\n", len(filteredRecords)-1) // Subtract 1 for header
	for serialNum, tests := range passingTests {
		uniqueTestIDs := make(map[string]bool)
		for _, test := range tests {
			uniqueTestIDs[test.testID] = true
		}
		keptTestIDs := len(selectedTestIDs[serialNum])
		fmt.Printf("Serial %s: %d unique passing test IDs found, %d kept (max %d)\n",
			serialNum,
			len(uniqueTestIDs),
			keptTestIDs,
			maxTestRuns)
	}

	// Now process the filtered data
	groupedTests := make(map[string][]TestRow)
	skippedRows := 0
	processedRows := 0

	// Track unique test IDs and passing tests per serial number
	type SerialStats struct {
		uniqueTests  map[string]bool
		passingTests map[string]bool
	}
	serialStats := make(map[string]*SerialStats)

	// Process filtered records
	for i, record := range filteredRecords[1:] { // Skip header
		if len(record) < len(headers) {
			log.Printf("Skipping row %d: incomplete data (expected %d columns, got %d)", i+2, len(headers), len(record))
			skippedRows++
			continue
		}

		// Track test IDs and passing tests per serial number
		serialNum := strings.TrimSpace(record[columnIndices["serial_name"]])
		testID := strings.TrimSpace(record[columnIndices["filename"]])
		testResult := strings.TrimSpace(record[columnIndices["result"]])

		if serialNum != "" && testID != "" {
			if _, exists := serialStats[serialNum]; !exists {
				serialStats[serialNum] = &SerialStats{
					uniqueTests:  make(map[string]bool),
					passingTests: make(map[string]bool),
				}
			}
			serialStats[serialNum].uniqueTests[testID] = true

			if strings.ToLower(testResult) == "pass" || strings.ToLower(testResult) == "true" {
				serialStats[serialNum].passingTests[testID] = true
			}
		}

		// Skip rows where param_value_float is empty
		if strings.TrimSpace(record[columnIndices["value"]]) == "" {
			skippedRows++
			continue
		}

		// Parse numeric values with better error handling
		value, err := strconv.ParseFloat(strings.TrimSpace(record[columnIndices["value"]]), 64)
		if err != nil {
			log.Printf("Skipping row %d: invalid value '%s': %v", i+2, record[columnIndices["value"]], err)
			skippedRows++
			continue
		}

		// Skip if lower or upper limit is empty
		if strings.TrimSpace(record[columnIndices["lower_limit"]]) == "" || strings.TrimSpace(record[columnIndices["upper_limit"]]) == "" {
			skippedRows++
			continue
		}

		lowerLimit, err := strconv.ParseFloat(strings.TrimSpace(record[columnIndices["lower_limit"]]), 64)
		if err != nil {
			log.Printf("Skipping row %d: invalid lower limit '%s': %v", i+2, record[columnIndices["lower_limit"]], err)
			skippedRows++
			continue
		}

		upperLimit, err := strconv.ParseFloat(strings.TrimSpace(record[columnIndices["upper_limit"]]), 64)
		if err != nil {
			log.Printf("Skipping row %d: invalid upper limit '%s': %v", i+2, record[columnIndices["upper_limit"]], err)
			skippedRows++
			continue
		}

		test := TestRow{
			parameterName: strings.TrimSpace(record[columnIndices["parameter_name"]]),
			Description:   strings.TrimSpace(record[columnIndices["description"]]),
			Comparator:    strings.TrimSpace(record[columnIndices["comparator"]]),
			Value:         value,
			LowerLimit:    lowerLimit,
			UpperLimit:    upperLimit,
			SerialName:    serialNum,
			Filename:      testID,
		}

		// Check if the test has valid data
		if test.parameterName == "" || test.Description == "" || test.SerialName == "" {
			log.Printf("Skipping row %d: missing required fields", i+2)
			skippedRows++
			continue
		}

		if test.Comparator == "GELE" {
			key := test.parameterName
			groupedTests[key] = append(groupedTests[key], test)
			processedRows++
		} else {
			skippedRows++
		}
	}

	// Print summary table
	fmt.Println("\nSerial Number Summary (Filtered Data):")
	fmt.Println("=====================================")
	fmt.Printf("%-20s %-15s %-15s\n", "Serial Number", "Total Tests", "Passing Tests")
	fmt.Println(strings.Repeat("-", 50))

	// Sort serial numbers for consistent output
	var serialNums []string
	for serialNum := range serialStats {
		serialNums = append(serialNums, serialNum)
	}
	sort.Strings(serialNums)

	for _, serialNum := range serialNums {
		stats := serialStats[serialNum]
		fmt.Printf("%-20s %-15d %-15d\n",
			serialNum,
			len(stats.uniqueTests),
			len(stats.passingTests))
	}
	fmt.Println(strings.Repeat("-", 50))

	fmt.Printf("\nProcessing complete:\n")
	fmt.Printf("- Total rows in filtered data: %d\n", len(filteredRecords)-1)
	fmt.Printf("- Successfully processed rows: %d\n", processedRows)
	fmt.Printf("- Skipped rows: %d\n", skippedRows)
	fmt.Printf("- Total unique serial numbers: %d\n", len(serialNums))

	// Sanity check: Verify consistent limits for each parameter
	type ParameterLimits struct {
		upperLimits map[float64]bool
		lowerLimits map[float64]bool
	}
	parameterChecks := make(map[string]*ParameterLimits)

	// First pass: collect all unique limits for each parameter
	for _, tests := range groupedTests {
		for _, test := range tests {
			key := test.parameterName
			if _, exists := parameterChecks[key]; !exists {
				parameterChecks[key] = &ParameterLimits{
					upperLimits: make(map[float64]bool),
					lowerLimits: make(map[float64]bool),
				}
			}
			parameterChecks[key].upperLimits[test.UpperLimit] = true
			parameterChecks[key].lowerLimits[test.LowerLimit] = true
		}
	}

	// Second pass: verify limits (only if force flag is not set)
	if !forceFlag {
		for paramKey, limits := range parameterChecks {
			if len(limits.upperLimits) > 1 {
				log.Fatalf("Inconsistent upper limits found for parameter %s: found %d distinct upper limits", paramKey, len(limits.upperLimits))
			}
			if len(limits.lowerLimits) > 1 {
				log.Fatalf("Inconsistent lower limits found for parameter %s: found %d distinct lower limits", paramKey, len(limits.lowerLimits))
			}
		}
	} else {
		// If force flag is set, just log a warning about inconsistent limits
		for paramKey, limits := range parameterChecks {
			if len(limits.upperLimits) > 1 {
				log.Printf("Warning: Inconsistent upper limits found for parameter %s: found %d distinct upper limits", paramKey, len(limits.upperLimits))
			}
			if len(limits.lowerLimits) > 1 {
				log.Printf("Warning: Inconsistent lower limits found for parameter %s: found %d distinct lower limits", paramKey, len(limits.lowerLimits))
			}
		}
	}

	if len(groupedTests) == 0 {
		log.Fatal("No valid test data found after processing")
	}

	// Write grouped results
	err = writeGroupedResults("grr_summary.csv", groupedTests)
	if err != nil {
		log.Fatalf("Error writing grouped results: %v", err)
	}
	fmt.Println("Grouped results written to grr_summary.csv")

	// Run TRV Analysis
	fmt.Println("\nRunning TRV Analysis...")
	cmd := exec.Command("python3", "TRVAnalysis.py")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("Warning: TRV Analysis failed: %v", err)
		log.Println("You may need to set up the Python virtual environment first.")
		log.Println("See README.md for instructions.")
	} else {
		fmt.Println("TRV Analysis completed successfully")
	}

	// Get actor name from tester_name column
	var actorName string
	var warnedAboutTesterNames bool
	for _, record := range records[1:] { // Skip header
		if len(record) < len(headers) {
			continue
		}
		testerName := strings.TrimSpace(record[columnIndices["tester_name"]])
		if testerName != "" {
			if actorName == "" {
				actorName = testerName
			} else if actorName != testerName && !warnedAboutTesterNames {
				log.Printf("Warning: Inconsistent tester names found: %s vs %s", actorName, testerName)
				warnedAboutTesterNames = true
				// Use the first encountered name
			}
		}
	}

	if actorName == "" {
		log.Fatal("Could not determine actor name from tester_name column")
	}

	// Create zip file
	zipName := fmt.Sprintf("grr_summary_%s.zip", actorName)
	zipFile, err := os.Create(zipName)
	if err != nil {
		log.Fatalf("Error creating zip file: %v", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Files to include in zip
	filesToZip := []string{
		"data_raw.csv",
		"data_filtered.csv",
		"grr_summary.csv",
	}

	// Add files to zip
	for _, file := range filesToZip {
		if err := addFileToZip(zipWriter, file); err != nil {
			log.Fatalf("Error adding %s to zip: %v", file, err)
		}
	}

	// Add scatter_plots directory
	if err := addDirToZip(zipWriter, "scatter_plots"); err != nil {
		log.Fatalf("Error adding scatter_plots to zip: %v", err)
	}

	fmt.Printf("\nCreated zip file: %s\n", zipName)
}

func getGitHash() (string, error) {
	cmd := exec.Command("git", "describe", "--tags", "--dirty", "--always", "--long")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error retrieving git hash: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func indexOf(headers []string, column string) int {
	for i, header := range headers {
		if header == column {
			return i
		}
	}
	return -1
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
		Count                  int
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
		parameterName := key

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
		grp := (gr / math.Abs(tests[0].UpperLimit-tests[0].LowerLimit)) * 100
		results = append(results, ResultRow{
			TaskDescription:        parameterName,
			Count:                  len(tests),
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
	err = writer.Write([]string{"parameter_name", "count", "lower_limit", "upper_limit", "Repeatability [units]", "Reproducability [units]", "TotalGRR [units]", "GRRTolerancePercentage [%]"})
	if err != nil {
		return fmt.Errorf("error writing header: %w", err)
	}

	// Write sorted data
	for _, row := range results {
		err = writer.Write([]string{
			row.TaskDescription,
			fmt.Sprintf("%d", row.Count),
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

// Helper function to add a file to zip
func addFileToZip(zipWriter *zip.Writer, filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filename
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, file)
	return err
}

// Helper function to add a directory to zip
func addDirToZip(zipWriter *zip.Writer, dirname string) error {
	return filepath.Walk(dirname, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Create relative path for zip
		relPath, err := filepath.Rel(".", path)
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = relPath
		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})
}
