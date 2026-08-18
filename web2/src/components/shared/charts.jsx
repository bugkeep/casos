import React from "react";
import {Bar, BarChart, Cell, Pie, PieChart, XAxis, YAxis} from "recharts";
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart";

/**
 * Blue-family multi-hue palette. The theme only ships five --chart-* slots and
 * the dashboard needs more series than that, so the palette is declared here
 * and is deliberately kept legible against both the light and the dark surface.
 */
export const CHART_COLORS = [
  "#3b82f6",
  "#0ea5e9",
  "#06b6d4",
  "#14b8a6",
  "#6366f1",
  "#8b5cf6",
  "#2563eb",
  "#0284c7",
  "#0891b2",
  "#0f766e",
  "#7c3aed",
  "#38bdf8",
  "#5eead4",
];

const RADIAN = Math.PI / 180;

function truncate(value, max) {
  const text = String(value ?? "");
  return text.length > max ? `${text.slice(0, max - 2)}…` : text;
}

// The palette lives in the config rather than on the marks so the legend and
// the tooltip can look a series' colour up by name.
function paletteConfig(entries, colors, offset = 0) {
  return Object.fromEntries(entries.map(({name}, index) => [
    name,
    {label: name, color: colors?.[name] || CHART_COLORS[(index + offset) % CHART_COLORS.length]},
  ]));
}

function toEntries(data) {
  return Object.entries(data || {}).map(([name, value]) => ({name, value}));
}

// Pie tooltips carry the share as well as the count, which is the number the
// donut is actually drawn from.
function shareFormatter(total) {
  return (value, name, item) => (
    <>
      <div
        className="h-2.5 w-2.5 shrink-0 rounded-[2px]"
        style={{backgroundColor: item?.payload?.fill || item?.color}}
      />
      <div className="flex flex-1 items-center justify-between gap-4 leading-none">
        <span className="text-muted-foreground">{name}</span>
        <span className="text-foreground font-mono font-medium tabular-nums">
          {value}{total > 0 ? ` (${((value / total) * 100).toFixed(1)}%)` : ""}
        </span>
      </div>
    </>
  );
}

/**
 * Category breakdown with the legend beside the ring rather than under it, so
 * the chart keeps its height in a short card.
 */
export function CategoryDonut({data, colors, className}) {
  const entries = toEntries(data);
  if (entries.length === 0) {
    return null;
  }
  const config = paletteConfig(entries, colors);
  const total = entries.reduce((sum, entry) => sum + entry.value, 0);

  return (
    <ChartContainer config={config} className={className}>
      <PieChart>
        <ChartTooltip content={<ChartTooltipContent hideLabel nameKey="name" formatter={shareFormatter(total)} />} />
        <Pie
          data={entries}
          dataKey="value"
          nameKey="name"
          cx="30%"
          cy="50%"
          innerRadius="55%"
          outerRadius="88%"
          paddingAngle={2}
          strokeWidth={2}
        >
          {entries.map((entry) => (
            <Cell key={entry.name} fill={config[entry.name].color} className="stroke-background" />
          ))}
        </Pie>
        <ChartLegend
          layout="vertical"
          align="right"
          verticalAlign="middle"
          content={<ChartLegendContent nameKey="name" className="flex-col items-start gap-1.5 pt-0" />}
        />
      </PieChart>
    </ChartContainer>
  );
}

/**
 * Horizontal bars for a "top N" ranking. A cluster can have hundreds of
 * namespaces, so the caller passes the slice worth looking at.
 */
export function RankedBarChart({data, limit = 12, valueLabel, className}) {
  const rows = toEntries(data)
    .sort((a, b) => b.value - a.value)
    .slice(0, limit);
  if (rows.length === 0) {
    return null;
  }
  const config = {value: {label: valueLabel}};

  return (
    <ChartContainer config={config} className={className}>
      <BarChart data={rows} layout="vertical" margin={{top: 8, right: 24, bottom: 8, left: 8}}>
        <XAxis type="number" allowDecimals={false} tickLine={false} axisLine={false} fontSize={11} />
        <YAxis
          type="category"
          dataKey="name"
          width={130}
          tickLine={false}
          axisLine={false}
          fontSize={11}
          tickFormatter={(value) => truncate(value, 20)}
        />
        <ChartTooltip content={<ChartTooltipContent />} />
        <Bar dataKey="value" radius={[0, 4, 4, 0]} maxBarSize={26}>
          {rows.map((row, index) => (
            <Cell key={row.name} fill={CHART_COLORS[index % CHART_COLORS.length]} />
          ))}
        </Bar>
      </BarChart>
    </ChartContainer>
  );
}

// Slice labels sit just outside the ring: the band of a thin donut is narrower
// than the words that would have to fit in it.
function renderSliceLabel({cx, cy, midAngle, outerRadius, percent, name}) {
  // A sliver's label lands on top of its neighbour's; the legend and the
  // tooltip still carry the name.
  if (percent < 0.08) {
    return null;
  }
  const x = cx + (outerRadius + 12) * Math.cos(-midAngle * RADIAN);
  const y = cy + (outerRadius + 12) * Math.sin(-midAngle * RADIAN);
  return (
    <text
      x={x}
      y={y}
      fontSize={11}
      textAnchor={x > cx ? "start" : "end"}
      dominantBaseline="central"
      className="fill-muted-foreground"
    >
      {truncate(name, 12)}
    </text>
  );
}

/**
 * Two rings side by side, for a pair of breakdowns of the same population —
 * nodes by OS next to nodes by architecture.
 */
export function DualCategoryPie({left, right, className}) {
  const leftEntries = toEntries(left);
  const rightEntries = toEntries(right);
  if (leftEntries.length === 0 && rightEntries.length === 0) {
    return null;
  }
  const config = {...paletteConfig(leftEntries), ...paletteConfig(rightEntries, null, 4)};
  // Both rings count the same nodes, so one total gives the share for either.
  const total = Math.max(
    leftEntries.reduce((sum, entry) => sum + entry.value, 0),
    rightEntries.reduce((sum, entry) => sum + entry.value, 0));

  const ring = (entries, cx, offset) => (
    <Pie
      data={entries}
      dataKey="value"
      nameKey="name"
      cx={cx}
      cy="45%"
      innerRadius="26%"
      outerRadius="52%"
      strokeWidth={2}
      labelLine={false}
      label={renderSliceLabel}
    >
      {entries.map((entry, index) => (
        <Cell
          key={entry.name}
          fill={CHART_COLORS[(index + offset) % CHART_COLORS.length]}
          className="stroke-background"
        />
      ))}
    </Pie>
  );

  return (
    <ChartContainer config={config} className={className}>
      <PieChart>
        <ChartTooltip content={<ChartTooltipContent hideLabel nameKey="name" formatter={shareFormatter(total)} />} />
        {ring(leftEntries, "30%", 0)}
        {ring(rightEntries, "70%", 4)}
        <ChartLegend content={<ChartLegendContent nameKey="name" className="flex-wrap" />} />
      </PieChart>
    </ChartContainer>
  );
}
