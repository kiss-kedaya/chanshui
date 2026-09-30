import "./globals.css";
import Sidebar from "@/components/Sidebar";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "掺水？— 网关黑盒检测",
  description: "一键检测：注入 token、模型家族、思考文本回传、智商评分",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="zh-CN">
      <body>
        <Sidebar />
        <main className="main">{children}</main>
      </body>
    </html>
  );
}
