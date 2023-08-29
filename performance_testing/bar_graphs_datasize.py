import pandas as pd
import matplotlib.pyplot as plt
import numpy as np
from matplotlib.lines import Line2D

# Data
methods = [0, 1, 2]
data_trasfered = [
    [1, 1.01, 1.01],
    [1.15, 1.33, 1.33],
    [1.32, 1.70, 1.70]
]


bar_color = (125/255,60/255,85/255)

write_rates = ['1m/s', '50m/s', '100m/s'] 

size_unit_conversion = 1 
size_unit = 'GB'

max_size = np.max(data_trasfered)

fig, ax = plt.subplots(figsize=(6, 4))

bar_width = 0.1
bar_spacing = 0.05 
x = np.arange(len(methods)) 

# Create bars for each dataset
for i, (ds_trasfered, write_rate) in enumerate(zip(data_trasfered, write_rates)):   
    x_position = x + i * (bar_width + bar_spacing)
    # Create bars for migration time with downtime portion
    ax.bar(x_position, [size_unit_conversion * x for x in ds_trasfered], width=bar_width, color=bar_color)

    # Add labels for write rates directly above the bars
    for xi, d_trasfered in zip(x_position, ds_trasfered):
      ax.text(xi, d_trasfered * size_unit_conversion + 0.004, f'{write_rate}', ha='center', va='bottom', fontsize=8)


# Customize the plot
ax.set_xlabel('# of snapshots')
ax.set_ylabel(f'Data size (in {size_unit})')
ax.set_title('Amount of trasfered Data')
ax.set_xticks(x + ((bar_width + bar_spacing) * (len(data_trasfered) - 1) / 2))
ax.set_xticklabels(methods, fontsize=10)
ax.set_ylim(0, max_size * size_unit_conversion + 0.01)
ax.grid(axis='y', linestyle='-', alpha=0.6, color='darkgrey')


ax.set_facecolor('#F5F5F5') 


y_margin = 0.45 
y_max_limit = max_size * size_unit_conversion + y_margin
ax.set_ylim(0, y_max_limit)


# Create custom legend
legend_elements = [
    Line2D([0], [0], color=bar_color, lw=8, label='Data trasfered'),
]
ax.legend(handles=legend_elements, fontsize=8, loc='upper left')

# Save and display the plot
plt.tight_layout()
plt.savefig('bar_graphs_datasize.png', dpi=300)
plt.show()