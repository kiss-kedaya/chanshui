"use client";

import { useEffect } from "react";
import { staleChunks } from "@/lib/build-guard";

const CHUNKS = 'script[src^="/_next/static/chunks/"]';

function chunkNames(root: ParentNode): string[] {
  return Array.from(root.querySelectorAll(CHUNKS))
    .map((node) => node.getAttribute("src") || "")
    .filter(Boolean);
}

// Old tabs would otherwise keep a pre-release UI, or 404 on chunks a new
// release removed. Reload once the server HTML asks for a chunk we lack.
export default function BuildGuard() {
  useEffect(() => {
    const loaded = chunkNames(document);
    let busy = false;
    const check = async () => {
      if (busy) return;
      busy = true;
      try {
        const res = await fetch(location.pathname + "?build=" + Date.now(), {
          cache: "no-store",
        });
        if (!res.ok) return;
        const doc = new DOMParser().parseFromString(await res.text(), "text/html");
        if (staleChunks(loaded, chunkNames(doc))) location.reload();
      } catch {
        // Offline or a slow origin: keep the page as is.
      } finally {
        busy = false;
      }
    };
    const timer = window.setInterval(check, 300000);
    window.addEventListener("focus", check);
    void check();
    return () => {
      window.clearInterval(timer);
      window.removeEventListener("focus", check);
    };
  }, []);
  return null;
}