# Macro Parameter GRR Tool

This tool processes test data to perform Gage Repeatability and Reproducibility (GRR) analysis on macro parameters.

## Overview

The tool:
1. Reads test data from CSV files.
2. Filters to keep the latest 6 passing tests per serial number.
3. Performs GRR analysis on the filtered data.
4. Generates a summary report.
5. Runs TRV Analysis on the filtered data.

> **Important Note**: This tool only analyzes parameters with GELE comparator. If parameters are improperly set up (e.g., using different comparators), they will be ignored in the analysis. This could potentially conceal issues with the tester. Always verify that all relevant parameters are properly configured with GELE comparator before running the analysis.

## Prerequisites

### Go Requirements
- Go 1.16 or later

### Python Requirements (for TRV Analysis)
- Python 3.8 or later
- pip (Python package manager)

## Setup

1. Clone the repository:
   ```bash
   git clone <repository-url>
   cd MacroParameterGRRTool
   ```

2. Set up the Python virtual environment:
   ```bash
   # Create virtual environment
   python3 -m venv venv

   # Activate virtual environment
   # On Windows:
   venv\Scripts\activate
   # On macOS/Linux:
   source venv/bin/activate

   # Install required packages
   pip install -r requirements.txt
   ```

## Usage

1. Place your test data CSV file in the same directory as the tool
2. Run the tool:
   ```bash
   go run main.go
   ```
3. The tool will generate:
   - `data_filtered.csv`: Contains the filtered test data
   - `grr_summary.csv`: Contains the GRR analysis results
   - TRV Analysis results (if Python environment is set up)

Note: If you see a warning about TRV Analysis failing, make sure you have:
1. Created and activated the Python virtual environment
2. Installed the required packages
3. Python 3.8 or later is installed on your system

## Input Data Format

The input CSV file should contain the following columns:
- `serial_num`: Serial number of the unit being tested
- `test_id`: Unique identifier for each test
- `test_start_time`: Timestamp of when the test started
- `result`: Test result ("pass" or "true" for passing tests)
- `parameter_name`: Name of the parameter being tested
- `description`: Description of the parameter
- `comparator`: Comparison operator (e.g., "GELE")
- `param_value_float`: Measured value
- `lower_limit`: Lower limit for the parameter
- `upper_limit`: Upper limit for the parameter

### Data Source

The input data (`data_raw.csv`) is generated using the following SQL query:

```sql
select
    *
from
    test_result_new trn
    join parameter_outcomes_new pon on trn.test_id = pon.test_id 
where
    trn.serial_num in ('CE251950AFA', 'CE251950AD2', 'CE251950A8B', 'CE251950A8C', 'CE251950AE4', 'CE251950AB5')
    and pon.comparator like 'GELE'
    and pon.lower_limit != pon.upper_limit 
    and trn.result like "pass"
```

### Important Requirements

1. **Constant Limits**: For each unique combination of `parameter_name` and `description`, the `upper_limit` and `lower_limit` values must be consistent across all tests. The tool will fail if it detects different limit values for the same parameter.

2. **Passing Tests**: The tool filters for passing tests only (where `result` is "pass").

3. **Latest Tests**: For each serial number, only the latest 6 unique passing test IDs are kept.

### d2 Constant

The current d2 value (2.353) is set for the standard GRR configuration of:
- 5 units
- 6 measurements per unit
- 1 operator

If your GRR configuration is different, you'll need to update the d2 constant in `main.go`. You can find the appropriate d2 value from the d2_Lookup table based on your specific configuration:
- Number of parts (units)
- Number of operators
- Number of measurements per part

To update the d2 constant:
1. Open `main.go`
2. Find the line: `const d2 = 2.353`
3. Replace 2.353 with the appropriate value from the d2_Lookup table
4. Save and recompile the tool

## Output

The tool generates two CSV files:

### data_filtered.csv
Contains the filtered test data, keeping:
- Only passing tests
- Latest 6 unique test IDs per serial number
- All parameter rows for the selected test IDs

### grr_summary.csv
Contains the GRR analysis results with columns:
- parameter_name-description: Combined parameter identifier
- lower_limit: Lower specification limit
- upper_limit: Upper specification limit
- Repeatability [units]: Measurement of variation when one operator measures the same part multiple times
- Reproducability [units]: Measurement of variation when different operators measure the same part
- TotalGRR [units]: Combined repeatability and reproducibility
- GRRTolerancePercentage [%]: GRR as a percentage of the tolerance range

### TRV Analysis Results
The TRV Analysis script processes the filtered data and generates additional analysis results.

## Building

To build the tool:
```bash
go build
```

## Troubleshooting

### TRV Analysis Issues
If you encounter issues with the TRV Analysis:
1. Make sure the Python virtual environment is activated
2. Verify that all required packages are installed:
   ```bash
   pip list
   ```
3. Check that Python 3.8 or later is installed:
   ```bash
   python --version
   ```
4. If needed, recreate the virtual environment:
   ```bash
   rm -rf venv
   python3 -m venv venv
   source venv/bin/activate  # or venv\Scripts\activate on Windows
   pip install -r requirements.txt
   ```
