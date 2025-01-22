import os
import pandas as pd
import matplotlib.pyplot as plt

# Define the CSV file path
file_path = 'raw_data.csv'

# Read the CSV file into a pandas DataFrame
df = pd.read_csv(file_path)

# Ensure required columns exist
required_columns = ['serial_name', 'parameter_name', 'description', 'value', 'lower_limit', 'upper_limit']
if not all(col in df.columns for col in required_columns):
    raise ValueError(f"The CSV file must contain the following columns: {required_columns}")

# Group by 'parameter_name' and 'description'
groups = df.groupby(['parameter_name', 'description'])

# Create a scatter plot for each combination of 'parameter_name' and 'description'
output_folder = 'scatter_plots'
os.makedirs(output_folder, exist_ok=True)

for (param, desc), group in groups:
    plt.figure(figsize=(10, 6))
    
    # Group by 'serial_name' and plot multiple values for each serial_name
    for serial_name, serial_group in group.groupby('serial_name'):
        x_positions = [serial_name] * len(serial_group)  # Same x position for this serial_name
        plt.scatter(x_positions, serial_group['value'], label=f'{serial_name}', alpha=0.7)
    
    # Add horizontal lines for 'upper_limit' and 'lower_limit'
    upper_limit = group['upper_limit'].iloc[0]  # Assuming limits are consistent within a group
    lower_limit = group['lower_limit'].iloc[0]
    plt.axhline(y=upper_limit, color='red', linestyle='--', label='Upper Limit')
    plt.axhline(y=lower_limit, color='red', linestyle='--', label='Lower Limit')

    # Set plot title and labels
    plt.title(f"Scatter Plot for {param} - {desc}")
    plt.xlabel("Serial Name")
    plt.ylabel("Value")
    plt.xticks(rotation=45, ha='right')  # Rotate x-axis labels for readability
    plt.legend()
    plt.grid(True, linestyle='--', alpha=0.5)

    # Save the plot
    filename = f"{param}_{desc}.png".replace(" ", "_")
    plt.tight_layout()
    plt.savefig(os.path.join(output_folder, filename))
    plt.close()

print(f"Scatter plots have been saved to the folder: {output_folder}")
