import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "公共检测接口",
  description: "掺水？公共检测 API。给 Agent 调用，每分钟 1 次，可指定板块。结果不进入全站检测历史。",
  alternates: { canonical: "https://chanshui.dev/docs" },
  keywords: ["检测站", "中转站", "降智", "模型", "API", "模型检测"],
};

export default function DocsLayout({ children }: { children: React.ReactNode }) {
  return children;
}
