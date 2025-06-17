import os
import pandas as pd
import numpy as np
import matplotlib.pyplot as plt
import re

# Define the CSV file path
file_path = 'data_filtered.csv'

# Bin size as fraction of tolerance range
BIN_SIZE_FRACTION = 1/20

def sanitize_filename(filename):
    # Replace any non-alphanumeric characters (except underscores) with underscores
    sanitized = re.sub(r'[^a-zA-Z0-9_]', '_', filename)
    # Replace multiple consecutive underscores with a single one
    sanitized = re.sub(r'_+', '_', sanitized)
    # Remove leading/trailing underscores
    sanitized = sanitized.strip('_')
    return sanitized

# Read the CSV file into a pandas DataFrame
df = pd.read_csv(file_path)

# Map the column names to match what the script expects
column_mapping = {
    'serial_num': 'serial_name',
    'param_value_float': 'value'
}

# Rename columns if they exist
for old_col, new_col in column_mapping.items():
    if old_col in df.columns:
        df = df.rename(columns={old_col: new_col})

# Ensure required columns exist
required_columns = ['serial_name', 'parameter_name', 'description', 'value', 'lower_limit', 'upper_limit']
if not all(col in df.columns for col in required_columns):
    missing_columns = [col for col in required_columns if col not in df.columns]
    raise ValueError(f"The CSV file is missing the following columns: {missing_columns}")

# Group by 'parameter_name' only and get the first description for each parameter
groups = df.groupby('parameter_name')
first_descriptions = df.groupby('parameter_name')['description'].first()

# Create a scatter plot and histogram for each parameter
output_folder = 'scatter_plots'
os.makedirs(output_folder, exist_ok=True)

for param, group in groups:
    # Get the first description for this parameter
    desc = first_descriptions[param]
    
    fig, axes = plt.subplots(1, 2, figsize=(16, 6), gridspec_kw={'width_ratios': [2, 1]})

    # Calculate the y-axis limits based on lower and upper limits
    upper_limit = group['upper_limit'].iloc[0]  # Assuming limits are consistent within a group
    lower_limit = group['lower_limit'].iloc[0]
    
    # Check for inverted limits and warn
    if lower_limit > upper_limit:
        print(f"Warning: Inverted limits found for {param}: lower_limit ({lower_limit}) > upper_limit ({upper_limit})")
        lower_limit, upper_limit = upper_limit, lower_limit
    
    y_min = lower_limit - (upper_limit - lower_limit) * 0.1
    y_max = upper_limit + (upper_limit - lower_limit) * 0.1

    # Scatter plot (left subplot)
    ax_scatter = axes[0]
    
    # Create a list of unique serial names for x-axis
    unique_serials = sorted(group['serial_name'].unique())
    serial_to_x = {serial: i for i, serial in enumerate(unique_serials)}
    
    for serial_name, serial_group in group.groupby('serial_name'):
        values = pd.to_numeric(serial_group['value'], errors='coerce')
        x_pos = serial_to_x[serial_name]
        
        # Filter the values that are within bounds
        in_bounds = values[(values <= y_max) & (values >= y_min)]
        x_positions = [x_pos] * len(in_bounds)

        # Plot in-bounds values
        ax_scatter.scatter(x_positions, in_bounds, alpha=0.7)
        
        # Mark points that are out of bounds
        out_of_bounds_top = values[values > y_max]
        out_of_bounds_bottom = values[values < y_min]

        for _ in out_of_bounds_top:
            ax_scatter.text(x_pos, y_max * 0.98, '^', fontsize=12, ha='center', va='bottom', color='blue')
        for _ in out_of_bounds_bottom:
            ax_scatter.text(x_pos, y_min * 1.02, 'v', fontsize=12, ha='center', va='top', color='blue')

    # Set x-axis ticks and labels
    ax_scatter.set_xticks(range(len(unique_serials)))
    ax_scatter.set_xticklabels(unique_serials, rotation=45)

    # Add horizontal lines for 'upper_limit' and 'lower_limit'
    ax_scatter.axhline(y=upper_limit, color='red', linestyle='--')
    ax_scatter.axhline(y=lower_limit, color='red', linestyle='--')

    # Set y-axis limits
    ax_scatter.set_ylim(y_min, y_max)

    # Set plot title and labels for scatter plot
    ax_scatter.set_title(f"Scatter Plot for {param} - {desc}")
    ax_scatter.set_xlabel("Serial Name")
    ax_scatter.set_ylabel("Value")
    ax_scatter.tick_params(axis='x', rotation=45)
    ax_scatter.grid(True, linestyle='--', alpha=0.5)

    # Histogram (right subplot)
    ax_hist = axes[1]
    
    # Convert values to numeric and handle any invalid values
    values = pd.to_numeric(group['value'], errors='coerce')
    values = values.dropna()  # Remove any NaN values
    
    if len(values) > 0:
        # Calculate bin size based on tolerance range
        tolerance_range = upper_limit - lower_limit
        bin_size = tolerance_range * BIN_SIZE_FRACTION
        
        # Calculate number of bins needed to cover the full range
        # Add extra bins on each side to show out-of-spec values
        range_to_cover = y_max - y_min
        num_bins = int(np.ceil(range_to_cover / bin_size))
        
        # Create bins
        bins = np.linspace(y_min, y_max, num_bins + 1)
        
        # Plot histogram
        ax_hist.hist(values, bins=bins, orientation='horizontal', color='gray', edgecolor='black', alpha=0.7)
        
        # Add horizontal lines for 'upper_limit' and 'lower_limit' on histogram
        ax_hist.axhline(y=upper_limit, color='red', linestyle='--')
        ax_hist.axhline(y=lower_limit, color='red', linestyle='--')
        ax_hist.set_ylim(y_min, y_max)
        
        # Set labels and title for histogram
        ax_hist.set_title("Histogram")
        ax_hist.set_xlabel("Frequency")
        ax_hist.set_ylabel("Value")
        ax_hist.grid(True, linestyle='--', alpha=0.5)
    else:
        ax_hist.text(0.5, 0.5, "No valid data for histogram", 
                    horizontalalignment='center', verticalalignment='center',
                    transform=ax_hist.transAxes)
        ax_hist.set_title("Histogram (No Data)")
        ax_hist.set_xlabel("Frequency")
        ax_hist.set_ylabel("Value")

    try:
        # Save the plot
        filename = f"{param}_{desc}.png"
        filename = sanitize_filename(filename)
        plt.tight_layout()
        plt.savefig(os.path.join(output_folder, filename))
        plt.close()
    except Exception as e:
        print(f"Error saving plot for {param} - {desc}: {str(e)}")

print(f"Scatter plots and histograms have been saved to the folder: {output_folder}")
