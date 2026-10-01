import { SVGProps } from "react";
export type IconName =
  | "arrow"
  | "scan"
  | "shield"
  | "layers"
  | "chart"
  | "clock"
  | "check"
  | "close"
  | "chevron"
  | "link"
  | "copy"
  | "key"
  | "search"
  | "spark"
  | "eye"
  | "eyeOff"
  | "refresh"
  | "info"
  | "feedback"
  | "spinner"
  | "sun"
  | "moon";
const paths: Record<IconName, string> = {
  sun: "M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5M17 12a5 5 0 1 1-10 0 5 5 0 0 1 10 0",
  moon: "M21 13A9 9 0 0 1 11 3a7 7 0 1 0 10 10",
  eye: "M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Zm13 0a3 3 0 1 1-6 0 3 3 0 0 1 6 0",
  eyeOff:
    "m3 3 18 18M10 5c6-1 11 7 11 7s-1 3-4 5M6 6c-3 2-4 6-4 6s3.5 7 10 7c1.5 0 3-.5 4-1M9 9a4 4 0 0 0 6 6",
  refresh:
    "M20 7v5h-5M4 17v-5h5M6 7a7 7 0 0 1 12-1l2 6M4 12l2 6a7 7 0 0 0 12-1",
  spinner: "M21 12a9 9 0 1 1-9-9",
  info: "M12 11v6m0-10v.01M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0",
  feedback:
    "M21 11a8 8 0 0 1-8 8H8l-5 3V7a4 4 0 0 1 4-4h10a4 4 0 0 1 4 4v4ZM7 8h10M7 12h7",
  arrow: "M5 12h14m-6-6 6 6-6 6",
  scan: "M8 3H5a2 2 0 0 0-2 2v3m13-5h3a2 2 0 0 1 2 2v3M3 16v3a2 2 0 0 0 2 2h3m8 0h3a2 2 0 0 0 2-2v-3M7 12h10",
  shield: "m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6l8-3Zm-4 9 3 3 5-6",
  layers: "m12 3 9 5-9 5-9-5 9-5Zm-9 10 9 5 9-5m-18 5 9 5 9-5",
  chart: "M4 4v16h16M8 15l4-5 4 3 4-7",
  clock: "M12 7v5l3 2M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0",
  check: "m5 12 4 4L19 6",
  close: "m6 6 12 12M6 18 18 6",
  chevron: "m8 4 8 8-8 8",
  link: "m10 13 4-4m-7 2-2 2a4 4 0 0 0 6 6l2-2m-2-10 2-2a4 4 0 0 1 6 6l-2 2",
  copy: "M8 8h12v13H8zM16 8V3H3v13h5",
  key: "M14 8a5 5 0 1 1-5-5 5 5 0 0 1 5 5Zm-1 4 8 8m-4-4 3-3",
  search: "M17 17l5 5M19 10a9 9 0 1 1-18 0 9 9 0 0 1 18 0",
  spark: "m12 3 2.5 6.5L21 12l-6.5 2.5L12 21l-2.5-6.5L3 12l6.5-2.5L12 3Z",
};
export default function Icon({
  name,
  ...props
}: { name: IconName } & SVGProps<SVGSVGElement>) {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...props}
    >
      <path d={paths[name]} />
    </svg>
  );
}
