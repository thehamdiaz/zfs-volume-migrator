import pandas as pd
import matplotlib.pyplot as plt
import numpy as np
from matplotlib.lines import Line2D

# Data
downtime_data = [
    (0, [105.217, 102.032, 103.876, 102.418, 101.450, 102.312, 102.946, 103.543, 105.877, 105.743]),
    (1, [21.378, 21.215, 22.155, 23.892, 22.505, 23.936, 22.678, 23.206, 24.001, 22.801]),
    (2, [23.102, 20.102, 20.102, 19.102, 22.102, 20.102, 22.102, 20.102, 20.102, 19.102])
]

migration_times_data = [(0, 107.705),(1, 117.893), (2, 122.123)]

averages = {}

for method, downtimes in downtime_data:
    avg_downtime = sum(downtimes) / len(downtimes)
    averages[method] = avg_downtime

print("Average Downtimes:")
for method, avg in averages.items():
    print(f"Method {method}: {avg:.3f} seconds")

ratio_0_to_1 = averages[0] / averages[1]
print(f"Ratio between Method 0 and Method 1: {ratio_0_to_1:.3f}")
ratio_0_to_2 = averages[0] / averages[2]
print(f"Ratio between Method 0 and Method 2: {ratio_0_to_2:.3f}")


methods = [item[0] for item in downtime_data]
downtimes = [item[1] for item in downtime_data]

# Create a figure and axis
fig, ax = plt.subplots(figsize=(12, 8))

# Create box plots
box = ax.boxplot(downtimes, patch_artist=True)

x_offsets = [0, 0, 0]
vertical_offsets = [0, -90, -96]

for i, (method, avg_migration_time) in enumerate(migration_times_data):
    method_index = methods.index(method) + 1
    x = method_index + x_offsets[i]
    y = avg_migration_time + vertical_offsets[i]
    plt.text(x, y, f'Avg Migration \n Time: {avg_migration_time:.3f}',
             ha='center', fontsize=10, color='black')

# Customizing the box plot colors
colors = ['lightblue', 'lightgreen', 'lightcoral']
for patch, color in zip(box['boxes'], colors):
    patch.set_facecolor(color)

ax.set_yticks(range(0, 160, 10))
max_size = max([item[1] for item in migration_times_data])
y_max_limit = max_size
ax.set_ylim(0, y_max_limit)

ax.set_xticklabels(['0', '1', '2'])
ax.set_ylabel('Downtime (Seconds)')
ax.set_xlabel('# snaphots')
plt.title('Box Plots of Downtimes for Different Methods')
ax.grid(axis='y', linestyle='-', alpha=0.6, color='darkgrey')

# Change the background color
ax.set_facecolor('#F5F5F5') 

# Show the plot
plt.show()
