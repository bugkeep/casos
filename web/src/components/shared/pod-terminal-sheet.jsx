import React, {useEffect, useRef, useState} from "react";
import {Terminal} from "xterm";
import {FitAddon} from "xterm-addon-fit";
import * as Setting from "@/Setting";
import {SimpleSelect} from "@/components/shared/simple-select";
import {ResourceSheet} from "@/components/shared/resource-sheet";
import "xterm/css/xterm.css";

// The exec websocket multiplexes two channels on one binary connection: a
// leading 0 byte marks stdin, a leading 1 byte marks a JSON resize message.
const CHANNEL_STDIN = 0;
const CHANNEL_RESIZE = 1;

function frame(channel, payload) {
  const encoded = new TextEncoder().encode(payload);
  const buffer = new Uint8Array(1 + encoded.length);
  buffer[0] = channel;
  buffer.set(encoded, 1);
  return buffer;
}

/** An interactive shell inside one container of a pod. */
export function PodTerminalSheet({pod, open, onClose}) {
  const mountRef = useRef(null);
  const termRef = useRef(null);
  const fitAddonRef = useRef(null);
  const socketRef = useRef(null);
  const [container, setContainer] = useState("");

  function cleanup() {
    if (socketRef.current) {
      socketRef.current.close();
      socketRef.current = null;
    }
    if (termRef.current) {
      termRef.current.dispose();
      termRef.current = null;
    }
  }

  function sendResize(cols, rows) {
    if (socketRef.current?.readyState === WebSocket.OPEN) {
      socketRef.current.send(frame(CHANNEL_RESIZE, JSON.stringify({cols, rows})));
    }
  }

  function openTerminal(namespace, name, containerName) {
    cleanup();

    const term = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: "'Cascadia Code', 'Fira Mono', Consolas, monospace",
      theme: {background: "#0a0a0a", foreground: "#d4d4d4", cursor: "#60a5fa"},
    });
    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    termRef.current = term;
    fitAddonRef.current = fitAddon;

    if (mountRef.current) {
      term.open(mountRef.current);
      fitAddon.fit();
    }

    const socket = new WebSocket(
      Setting.getWebSocketUrl("/api/pod-terminal", {namespace, name, container: containerName})
    );
    socket.binaryType = "arraybuffer";
    socketRef.current = socket;

    socket.onopen = () => sendResize(term.cols, term.rows);

    socket.onmessage = (event) => {
      if (termRef.current !== term) {
        return;
      }
      term.write(typeof event.data === "string" ? event.data : new Uint8Array(event.data));
    };

    socket.onclose = () => {
      if (termRef.current === term) {
        term.write("\r\n\x1b[31m[connection closed]\x1b[0m\r\n");
      }
    };

    socket.onerror = () => {
      if (termRef.current === term) {
        term.write("\r\n\x1b[31m[websocket error]\x1b[0m\r\n");
      }
    };

    term.onData((data) => {
      if (socketRef.current?.readyState === WebSocket.OPEN) {
        socketRef.current.send(frame(CHANNEL_STDIN, data));
      }
    });

    term.onResize(({cols, rows}) => sendResize(cols, rows));
  }

  useEffect(() => {
    if (!open || !pod) {
      cleanup();
      return undefined;
    }
    const defaultContainer = pod.containers?.[0] ?? "";
    setContainer(defaultContainer);
    // xterm measures the element to pick its grid size, so the terminal is
    // opened after the sheet has finished sliding in — measuring mid-animation
    // gives a terminal sized for a partially open pane.
    const timer = setTimeout(() => openTerminal(pod.namespace, pod.name, defaultContainer), 250);
    return () => {
      clearTimeout(timer);
      cleanup();
    };
  }, [open, pod]);

  useEffect(() => {
    if (!open) {
      return undefined;
    }
    const handleResize = () => fitAddonRef.current?.fit();
    window.addEventListener("resize", handleResize);
    return () => window.removeEventListener("resize", handleResize);
  }, [open]);

  function switchContainer(next) {
    setContainer(next);
    if (pod) {
      openTerminal(pod.namespace, pod.name, next);
    }
  }

  const containerOptions = (pod?.containers ?? []).map((name) => ({label: name, value: name}));

  return (
    <ResourceSheet
      open={open}
      onOpenChange={(next) => (next ? null : onClose())}
      title={pod ? `Terminal — ${pod.namespace} / ${pod.name}` : "Terminal"}
      size="xl"
      bodyClassName="bg-neutral-950 p-3"
      toolbar={
        containerOptions.length > 1 ? (
          <SimpleSelect
            value={container}
            onChange={switchContainer}
            options={containerOptions}
            size="sm"
            className="w-44"
            placeholder="Container"
          />
        ) : null
      }
    >
      <div ref={mountRef} className="min-h-0 flex-1" />
    </ResourceSheet>
  );
}

export default PodTerminalSheet;
