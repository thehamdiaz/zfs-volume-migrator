import pandas as pd
import matplotlib.pyplot as plt
import numpy as np
from matplotlib.lines import Line2D

# Data
methods = [0, 1, 2]
migration_times = [
    [109.705, 117.893, 122.123],
    [122.565, 172.952, 191.632],
    [155.630, 231.915, 241.095]
]
downtimes = [
    [105.892, 22.007, 20.102],
    [121.146, 51.64, 26.202],
    [139.408, 117.997, 31.343]
]
write_rates = ['1m/s', '50m/s', '100m/s'] 


time_unit_conversion = 1 
time_unit = 'seconds'


max_time = max(np.max(migration_times), np.max(downtimes))

fig, ax = plt.subplots(figsize=(6, 4))

# Adjust the bar width and spacing
bar_width = 0.1
bar_spacing = 0.05
x = np.arange(len(methods))

# Create bars for each dataset
for i, (m_times, d_times, write_rate) in enumerate(zip(migration_times, downtimes, write_rates)):
    x_position = x + i * (bar_width + bar_spacing)

    # Create bars for migration time with downtime portion
    ax.bar(x_position, m_times * time_unit_conversion, label=f'Migration Time - Snapshot {i}', width=bar_width, color='teal')
    ax.bar(x_position, d_times * time_unit_conversion, width=bar_width, color=(1, 0.3, 0.3))

    # Add labels for write rates directly above the bars
    for xi, (m_time, d_time) in zip(x_position, zip(m_times, d_times)):
      ax.text(xi, (m_time) * time_unit_conversion + 4, f'{write_rate}', ha='center', va='bottom', fontsize=8)

# Customize the plot
ax.set_xlabel('# of snapshots')
ax.set_ylabel(f'Time (in {time_unit})')
ax.set_title('Migration Time and Downtime')
ax.set_xticks(x + ((bar_width + bar_spacing) * (len(migration_times) - 1) / 2))
ax.set_xticklabels(methods, fontsize=10)
ax.set_ylim(0, max_time * time_unit_conversion + 10)
ax.grid(axis='y', linestyle='-', alpha=0.6, color='darkgrey')
ax.set_facecolor('#F5F5F5')


y_margin = 50
y_max_limit = max_time * time_unit_conversion + y_margin
ax.set_ylim(0, y_max_limit)


legend_elements = [
    Line2D([0], [0], color='teal', marker='s', markersize=7, markerfacecolor=(1, 0.3, 0.3), markevery=0, lw=8, label='Migration time'),
    Line2D([0], [0], color=(1, 0.3, 0.3), lw=8, label='Downtime')
]

# Add the legend
ax.legend(handles=legend_elements, fontsize=8, loc='upper left')

# Save and display the plot
plt.tight_layout()
plt.savefig('bar_graphs_times.png', dpi=300)
plt.show()