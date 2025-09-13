import json
import sys

import seaborn as sns
import matplotlib.pyplot as plt

def get_data():
	data = sys.stdin.read()
	results = json.loads(data)
	x_labels = []
	y_labels = []
	for result in results:
		parameter = result["parameter"]
		if parameter not in x_labels:
			x_labels.append(parameter)
		tag = result["tag"]
		if tag not in y_labels:
			y_labels.append(tag)
	data = []
	i = 0
	for y in y_labels:
		row = []
		for x in x_labels:
			result = results[i]
			sharpe_ratio = result["sharpeRatio"]
			row.append(sharpe_ratio)
			i += 1
		data.append(row)
	return x_labels, y_labels, data

def render_heatmap():
	x_labels, y_labels, data = get_data()
	plt.figure(figsize=(12, 8))
	sns.heatmap(
		data, 
		xticklabels=x_labels, 
		yticklabels=y_labels, 
		annot=True,
		fmt=".2f",
		cmap="viridis"
	)
	plt.xlabel("Range")
	plt.ylabel("Tag")
	plt.title("Sharpe Ratio")
	plt.xticks(rotation=45, ha="right", fontsize=10)
	plt.yticks(rotation=0, fontsize=10)
	plt.tight_layout()
	plt.show()

render_heatmap()