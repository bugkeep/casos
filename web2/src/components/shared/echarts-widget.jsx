import React, {useEffect, useRef} from "react";
import * as echarts from "echarts";
import {cn} from "@/lib/utils";

/**
 * Blue-family multi-hue palette. ECharts needs literal colours rather than CSS
 * variables, so the chart palette is declared here instead of in index.css and
 * is deliberately kept legible against both the light and the dark surface.
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

export function isDarkMode() {
  return document.documentElement.classList.contains("dark");
}

/**
 * ECharts container that owns the instance lifecycle: it resizes with its box,
 * and rebuilds when the theme flips, because a chart's axis and label colours
 * are baked into the instance rather than inherited from CSS.
 */
export function EchartsWidget({option, className, style}) {
  const containerRef = useRef(null);
  const chartRef = useRef(null);

  useEffect(() => {
    if (!containerRef.current) {
      return undefined;
    }

    function build() {
      chartRef.current?.dispose();
      const chart = echarts.init(containerRef.current, isDarkMode() ? "dark" : undefined, {renderer: "canvas"});
      // The echarts "dark" theme paints its own near-black background, which
      // would sit as a rectangle inside an already-dark card.
      chart.setOption({backgroundColor: "transparent"});
      chartRef.current = chart;
      if (option) {
        chart.setOption(option, {notMerge: true});
      }
    }

    build();

    const resizeObserver = new ResizeObserver(() => chartRef.current?.resize());
    resizeObserver.observe(containerRef.current);

    const themeObserver = new MutationObserver(build);
    themeObserver.observe(document.documentElement, {attributes: true, attributeFilter: ["class"]});

    return () => {
      resizeObserver.disconnect();
      themeObserver.disconnect();
      chartRef.current?.dispose();
      chartRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (chartRef.current && option) {
      chartRef.current.setOption(option, {notMerge: true});
    }
  }, [option]);

  return <div ref={containerRef} className={cn("w-full", className)} style={style} />;
}

export default EchartsWidget;
