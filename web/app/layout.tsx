import "./globals.css";
import ThemeMenu from "@/components/ThemeMenu";
import Sidebar from "@/components/Sidebar";
import type { Metadata } from "next";

export const metadata: Metadata = {
  metadataBase: new URL("https://chanshui.dev"),
  title: {
    default: "掺水？模型检测站｜中转站降智与接口审计",
    template: "%s｜掺水？模型检测站",
  },
  description:
    "掺水？模型检测站：面向模型中转站的降智检测、注入大小、缓存读写、工具调用、联网证据与推理表现审计。",
  keywords: [
    "检测站",
    "中转站",
    "降智",
    "模型",
    "模型检测站",
    "模型中转站",
    "接口审计",
    "AI模型检测",
  ],
  alternates: { canonical: "https://chanshui.dev/" },
  openGraph: {
    type: "website",
    url: "https://chanshui.dev/",
    siteName: "掺水？模型检测站",
    title: "掺水？模型检测站｜中转站降智与接口审计",
    description:
      "检测模型中转站的注入大小、缓存链、工具调用、联网证据与推理表现。",
    locale: "zh_CN",
  },
  robots: { index: true, follow: true },
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="zh-CN" suppressHydrationWarning>
      <head>
        <meta httpEquiv="Content-Type" content="text/html; charset=utf-8" />
        <script
          dangerouslySetInnerHTML={{
            __html:
              "try{const t=JSON.parse(localStorage.getItem('chanshui-theme')||'{}');document.documentElement.dataset.accent=t.accent==='mint'?'mint':'orange';document.documentElement.dataset.mode=t.mode==='dark'?'dark':'light'}catch{}",
          }}
        />
      </head>
      <body>
        <a href="#main-content" className="skip-link">
          跳到主内容
        </a>
        <Sidebar />
        <main className="main" id="main-content">
          <ThemeMenu />
          {children}
        </main>
      </body>
    </html>
  );
}
