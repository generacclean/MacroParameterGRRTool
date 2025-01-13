# GRR Analysis Tool

This Go application processes multiple CSV files containing test data, performs a GRR (Gage Repeatability and Reproducibility) analysis, and generates a summarized output CSV file (`grr_summary.csv`) with the results sorted in descending order of GRR percentage.

## Features

- Aggregates test data from multiple CSV files in the working directory.
- Groups tests by task name and description.
- Calculates GRR percentage based on repeatability.
- Outputs the results to `grr_summary.csv`, sorted by `grr_percentage` in descending order.

## Requirements

- Go 1.18 or later.
- CSV files must contain the following columns:
  - `task_name`
  - `description`
  - `comparator`
  - `value`
  - `lower_limit`
  - `upper_limit`

## Installation

1. Clone the repository:

   ```bash
   git clone https://github.com/yourusername/grr-analysis-tool.git
   cd grr-analysis-tool
   ```
2. Build the application:

    ```bash
    go build -o grr-analysis
    ```
## Output

The `grr_summary.csv` file contains the following columns:
- `task_name-description`: Combined task name and description.
- `lower_limit`: Lower limit of the test.
- `upper_limit`: Upper limit of the test.
- `grr_percentage`: Calculated GRR percentage.

### Example Output

| task_name-description | lower_limit | upper_limit | grr_percentage |
|------------------------|-------------|-------------|----------------|
| Task1-DescriptionA    | 0.50        | 1.50        | 12.34          |
| Task2-DescriptionB    | 1.00        | 2.00        | 10.56          |
| Task3-DescriptionC    | 0.25        | 1.25        | 8.78           |
