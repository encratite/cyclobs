import json
import sys

import pandas as pd
import seaborn as sns
import matplotlib.pyplot as plt
import matplotlib.ticker as mticker
import matplotlib.dates as mdates

def render_heatmap():
	x_labels, y_labels, data = get_heatmap_data()
	width = 1.1 * len(x_labels)
	height = 0.9 * len(y_labels)
	plt.figure(figsize=(width, height))
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

def get_heatmap_data():
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

def render_equity_curve():
	x, y = get_equity_curve_data()
	plt.figure(figsize=(12, 8))
	sns.lineplot(x=x, y=y)
	plt.title("Equity Curve")
	plt.xlabel("Date")
	plt.ylabel("Cash")
	ax = plt.gca()
	ax.set_xlim(min(x), max(x))	
	x_formatter = mdates.DateFormatter("%Y-%m-%d")
	ax.xaxis.set_major_formatter(x_formatter)
	ax.xaxis.set_major_locator(mdates.MonthLocator(interval=2))
	y_formatter = mticker.StrMethodFormatter("${x:,.0f}")
	ax.yaxis.set_major_formatter(y_formatter)
	plt.show()

def get_equity_curve_data():
	data = sys.stdin.read()
	samples = json.loads(data)
	x = []
	y = []
	for sample in samples:
		date = pd.to_datetime(sample["date"])
		cash = sample["cash"]
		x.append(date)
		y.append(cash)
	return x, y

argument = sys.argv[1]
if argument == "heatmap":
	render_heatmap()
elif argument == "equity":
	render_equity_curve()
else:
	print(f"Unknown argument \"{argument}\"")