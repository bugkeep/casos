import React, {useCallback, useEffect, useId, useLayoutEffect, useRef, useState} from "react";
import {cn} from "@/lib/utils";

/**
 * Force-directed graph drawn as plain SVG.
 *
 * The layout is a small velocity-Verlet simulation in the shape of d3-force:
 * every pair of nodes repels, every link pulls towards a rest length, and a
 * weak pull towards the centre keeps disconnected components from drifting off.
 * Positions are mutated in place and written straight to the DOM on each frame,
 * so a few hundred nodes settle without React re-rendering per tick.
 */

// Tuned against the shape a namespace actually has: a handful of controllers
// fanning out into pods, where a longer rest length is what keeps the labels
// from stacking on top of each other.
const CHARGE = -1000;
const LINK_DISTANCE = 140;
const CENTER_STRENGTH = 0.05;
const VELOCITY_DECAY = 0.6;
const ALPHA_MIN = 0.005;
const ALPHA_DECAY = 0.03;
// Reheat far enough to let neighbours follow a dragged node, but not so far
// that the whole graph re-shuffles under the cursor.
const DRAG_ALPHA = 0.3;
const MAX_ZOOM = 4;
const MIN_ZOOM = 0.2;
const FIT_PADDING = 48;
// A small graph is allowed to grow into the empty space, but only so far: past
// this the nodes read as blobs rather than as a diagram.
const MAX_FIT_ZOOM = 1.5;

function nodeRadius(node) {
  return (node.size || 30) / 2;
}

function buildSimulation(nodes, links, width, height) {
  const cx = width / 2;
  const cy = height / 2;
  const spread = Math.min(width, height) / 3;
  // A deterministic spiral beats Math.random() here: the same namespace lays
  // out the same way twice, so a re-fetch does not reshuffle the picture.
  const simNodes = nodes.map((node, index) => {
    const angle = index * 2.399963;
    const radius = spread * Math.sqrt((index + 1) / (nodes.length + 1));
    return {...node, x: cx + radius * Math.cos(angle), y: cy + radius * Math.sin(angle), vx: 0, vy: 0};
  });

  const byId = new Map(simNodes.map((node) => [node.id, node]));
  const simLinks = [];
  const neighbours = new Map(simNodes.map((node) => [node.id, new Set([node.id])]));
  const degree = new Map();
  links.forEach((link) => {
    const source = byId.get(link.source);
    const target = byId.get(link.target);
    // A link can name a resource that is not in this namespace's slice — an
    // ingress pointing at a service that no longer exists, say.
    if (!source || !target) {
      return;
    }
    simLinks.push({source, target});
    degree.set(source.id, (degree.get(source.id) || 0) + 1);
    degree.set(target.id, (degree.get(target.id) || 0) + 1);
    neighbours.get(source.id).add(target.id);
    neighbours.get(target.id).add(source.id);
  });

  // A deployment with forty pods hanging off it would be yanked forty times
  // per tick without this: the pull is divided by the busier endpoint's degree,
  // and the bias hands most of the correction to the leaf.
  simLinks.forEach((link) => {
    const sourceDegree = degree.get(link.source.id);
    const targetDegree = degree.get(link.target.id);
    link.strength = 1 / Math.min(sourceDegree, targetDegree);
    link.bias = sourceDegree / (sourceDegree + targetDegree);
  });

  return {nodes: simNodes, links: simLinks, neighbours};
}

function tick(simulation, alpha, width, height) {
  const {nodes, links} = simulation;
  const cx = width / 2;
  const cy = height / 2;

  for (let i = 0; i < nodes.length; i++) {
    const a = nodes[i];
    for (let j = i + 1; j < nodes.length; j++) {
      const b = nodes[j];
      let dx = b.x - a.x;
      let dy = b.y - a.y;
      let d2 = dx * dx + dy * dy;
      // Two nodes exactly on top of each other have no direction to push
      // apart in, so give them one.
      if (d2 < 1) {
        dx = ((i * 31 + j * 17) % 11) - 5;
        dy = ((i * 13 + j * 7) % 11) - 5;
        d2 = dx * dx + dy * dy || 1;
      }
      // CHARGE is negative, so each node is pushed along the vector that points
      // away from the other one.
      const force = (CHARGE * alpha) / d2;
      a.vx += dx * force;
      a.vy += dy * force;
      b.vx -= dx * force;
      b.vy -= dy * force;
    }
  }

  links.forEach(({source, target, strength, bias}) => {
    const dx = target.x - source.x;
    const dy = target.y - source.y;
    const distance = Math.sqrt(dx * dx + dy * dy) || 1;
    const force = ((distance - LINK_DISTANCE) / distance) * alpha * strength;
    source.vx += dx * force * (1 - bias);
    source.vy += dy * force * (1 - bias);
    target.vx -= dx * force * bias;
    target.vy -= dy * force * bias;
  });

  nodes.forEach((node) => {
    node.vx += (cx - node.x) * CENTER_STRENGTH * alpha;
    node.vy += (cy - node.y) * CENTER_STRENGTH * alpha;
    if (node.fixed) {
      node.vx = 0;
      node.vy = 0;
      return;
    }
    node.vx *= VELOCITY_DECAY;
    node.vy *= VELOCITY_DECAY;
    node.x += node.vx;
    node.y += node.vy;
  });
}

function fitTransform(nodes, width, height) {
  if (nodes.length === 0) {
    return {k: 1, x: 0, y: 0};
  }
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  nodes.forEach((node) => {
    const r = nodeRadius(node) + 18;
    minX = Math.min(minX, node.x - r);
    minY = Math.min(minY, node.y - r);
    maxX = Math.max(maxX, node.x + r);
    maxY = Math.max(maxY, node.y + r);
  });
  const spanX = Math.max(maxX - minX, 1);
  const spanY = Math.max(maxY - minY, 1);
  const k = Math.max(
    MIN_ZOOM,
    Math.min(MAX_FIT_ZOOM, (width - FIT_PADDING) / spanX, (height - FIT_PADDING) / spanY));
  return {
    k,
    x: width / 2 - k * (minX + spanX / 2),
    y: height / 2 - k * (minY + spanY / 2),
  };
}

export function ForceGraph({nodes, links, className, onNodeClick, getTooltip}) {
  const markerId = `arrow-${useId().replace(/:/g, "")}`;
  const containerRef = useRef(null);
  const viewRef = useRef(null);
  const nodeElements = useRef(new Map());
  const linkElements = useRef(new Map());
  const simulationRef = useRef({nodes: [], links: [], neighbours: new Map()});
  const transformRef = useRef({k: 1, x: 0, y: 0});
  const sizeRef = useRef({width: 0, height: 0});
  const alphaRef = useRef(0);
  const frameRef = useRef(0);
  const autoFitRef = useRef(true);
  const dragRef = useRef(null);
  const movedRef = useRef(false);

  const [, forceRender] = useState(0);
  const [hovered, setHovered] = useState(null);

  // Positions live outside React, so both the animation loop and a re-render
  // funnel through the same write.
  const draw = useCallback(() => {
    const {k, x, y} = transformRef.current;
    viewRef.current?.setAttribute("transform", `translate(${x},${y}) scale(${k})`);
    simulationRef.current.nodes.forEach((node) => {
      const element = nodeElements.current.get(node.id);
      element?.setAttribute("transform", `translate(${node.x},${node.y})`);
    });
    simulationRef.current.links.forEach((link, index) => {
      const element = linkElements.current.get(index);
      if (!element) {
        return;
      }
      // Both ends stop at the rim rather than the centre, so the arrowhead
      // stays visible instead of hiding under the target node.
      const dx = link.target.x - link.source.x;
      const dy = link.target.y - link.source.y;
      const distance = Math.sqrt(dx * dx + dy * dy) || 1;
      const from = nodeRadius(link.source) / distance;
      const to = (nodeRadius(link.target) + 3) / distance;
      element.setAttribute("x1", link.source.x + dx * from);
      element.setAttribute("y1", link.source.y + dy * from);
      element.setAttribute("x2", link.target.x - dx * to);
      element.setAttribute("y2", link.target.y - dy * to);
    });
  }, []);

  const run = useCallback(() => {
    cancelAnimationFrame(frameRef.current);
    const step = () => {
      const {width, height} = sizeRef.current;
      if (alphaRef.current <= ALPHA_MIN || width === 0) {
        return;
      }
      tick(simulationRef.current, alphaRef.current, width, height);
      alphaRef.current -= alphaRef.current * ALPHA_DECAY;
      if (autoFitRef.current) {
        transformRef.current = fitTransform(simulationRef.current.nodes, width, height);
      }
      draw();
      frameRef.current = requestAnimationFrame(step);
    };
    frameRef.current = requestAnimationFrame(step);
  }, [draw]);

  useLayoutEffect(() => {
    const container = containerRef.current;
    if (!container) {
      return undefined;
    }
    const observer = new ResizeObserver(([entry]) => {
      const {width, height} = entry.contentRect;
      const first = sizeRef.current.width === 0;
      sizeRef.current = {width, height};
      if (first && nodes.length > 0) {
        simulationRef.current = buildSimulation(nodes, links, width, height);
        alphaRef.current = 1;
        forceRender((value) => value + 1);
        run();
      }
    });
    observer.observe(container);
    return () => observer.disconnect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const {width, height} = sizeRef.current;
    if (width === 0) {
      return undefined;
    }
    nodeElements.current.clear();
    linkElements.current.clear();
    simulationRef.current = buildSimulation(nodes, links, width, height);
    alphaRef.current = 1;
    autoFitRef.current = true;
    setHovered(null);
    forceRender((value) => value + 1);
    run();
    return () => cancelAnimationFrame(frameRef.current);
  }, [nodes, links, run]);

  // A re-render (hover, new data) resets the attributes React does not own, so
  // the current positions are written back before the browser paints.
  useLayoutEffect(draw);

  useEffect(() => () => cancelAnimationFrame(frameRef.current), []);

  function toGraphPoint(event) {
    const rect = containerRef.current.getBoundingClientRect();
    const {k, x, y} = transformRef.current;
    return {
      x: (event.clientX - rect.left - x) / k,
      y: (event.clientY - rect.top - y) / k,
    };
  }

  function handlePointerDown(event, node) {
    event.currentTarget.setPointerCapture?.(event.pointerId);
    autoFitRef.current = false;
    movedRef.current = false;
    const point = toGraphPoint(event);
    dragRef.current = node
      ? {node, dx: node.x - point.x, dy: node.y - point.y}
      : {pan: true, x: event.clientX, y: event.clientY, origin: {...transformRef.current}};
    if (node) {
      node.fixed = true;
    }
  }

  function handlePointerMove(event) {
    const drag = dragRef.current;
    if (!drag) {
      return;
    }
    movedRef.current = true;
    if (drag.pan) {
      transformRef.current = {
        ...transformRef.current,
        x: drag.origin.x + (event.clientX - drag.x),
        y: drag.origin.y + (event.clientY - drag.y),
      };
      draw();
      return;
    }
    const point = toGraphPoint(event);
    drag.node.x = point.x + drag.dx;
    drag.node.y = point.y + drag.dy;
    alphaRef.current = Math.max(alphaRef.current, DRAG_ALPHA);
    draw();
    run();
  }

  function handlePointerUp() {
    if (dragRef.current?.node) {
      dragRef.current.node.fixed = false;
    }
    dragRef.current = null;
  }

  const handleWheel = useCallback((event) => {
    event.preventDefault();
    autoFitRef.current = false;
    const rect = containerRef.current.getBoundingClientRect();
    const {k, x, y} = transformRef.current;
    const next = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, k * Math.exp(-event.deltaY * 0.0015)));
    const px = event.clientX - rect.left;
    const py = event.clientY - rect.top;
    transformRef.current = {
      k: next,
      x: px - ((px - x) / k) * next,
      y: py - ((py - y) / k) * next,
    };
    draw();
  }, [draw]);

  // React attaches wheel listeners passively, where preventDefault is a no-op,
  // so zooming the graph would scroll the page as well.
  useEffect(() => {
    const container = containerRef.current;
    container?.addEventListener("wheel", handleWheel, {passive: false});
    return () => container?.removeEventListener("wheel", handleWheel);
  }, [handleWheel]);

  const {nodes: simNodes, links: simLinks, neighbours} = simulationRef.current;
  const adjacency = hovered ? neighbours.get(hovered.id) : null;
  const tooltip = hovered && getTooltip ? getTooltip(hovered) : null;

  return (
    <div
      ref={containerRef}
      className={cn("relative touch-none overflow-hidden select-none", className)}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerLeave={handlePointerUp}
    >
      <svg
        className="h-full w-full cursor-grab active:cursor-grabbing"
        onPointerDown={(event) => handlePointerDown(event, null)}
      >
        <defs>
          <marker
            id={markerId}
            viewBox="0 0 10 10"
            refX="10"
            refY="5"
            markerWidth="5"
            markerHeight="5"
            orient="auto-start-reverse"
          >
            <path d="M 0 0 L 10 5 L 0 10 z" className="fill-muted-foreground" />
          </marker>
        </defs>
        <g ref={viewRef}>
          {simLinks.map((link, index) => {
            const dimmed = hovered && link.source.id !== hovered.id && link.target.id !== hovered.id;
            return (
              <line
                key={`${link.source.id}->${link.target.id}-${index}`}
                ref={(element) => {
                  if (element) {
                    linkElements.current.set(index, element);
                  } else {
                    linkElements.current.delete(index);
                  }
                }}
                stroke={link.source.color || "currentColor"}
                strokeWidth={dimmed ? 1 : 1.5}
                strokeOpacity={dimmed ? 0.12 : 0.55}
                markerEnd={`url(#${markerId})`}
                className="text-muted-foreground"
              />
            );
          })}
          {simNodes.map((node) => {
            const radius = nodeRadius(node);
            const dimmed = adjacency && !adjacency.has(node.id);
            return (
              <g
                key={node.id}
                ref={(element) => {
                  if (element) {
                    nodeElements.current.set(node.id, element);
                  } else {
                    nodeElements.current.delete(node.id);
                  }
                }}
                className={cn("cursor-pointer", dimmed && "opacity-20")}
                onPointerDown={(event) => {
                  event.stopPropagation();
                  handlePointerDown(event, node);
                }}
                onPointerEnter={() => setHovered(node)}
                onPointerLeave={() => setHovered(null)}
                onClick={() => {
                  if (!movedRef.current) {
                    onNodeClick?.(node);
                  }
                }}
              >
                <circle
                  r={radius}
                  fill={node.color}
                  className={cn("stroke-background", hovered?.id === node.id && "stroke-foreground")}
                  strokeWidth={hovered?.id === node.id ? 2 : 1}
                />
                <text
                  y={radius + 12}
                  textAnchor="middle"
                  fontSize={11}
                  className="fill-muted-foreground pointer-events-none"
                >
                  {node.name.length > 22 ? `${node.name.slice(0, 20)}…` : node.name}
                </text>
              </g>
            );
          })}
        </g>
      </svg>
      {tooltip ? (
        <div
          className="border-border/50 bg-background pointer-events-none absolute z-10 rounded-lg border px-2.5 py-1.5 text-xs shadow-xl"
          style={{
            left: Math.min(
              transformRef.current.x + hovered.x * transformRef.current.k + 12,
              Math.max(sizeRef.current.width - 220, 0)),
            top: transformRef.current.y + hovered.y * transformRef.current.k + 12,
          }}
        >
          {tooltip}
        </div>
      ) : null}
    </div>
  );
}

export default ForceGraph;
