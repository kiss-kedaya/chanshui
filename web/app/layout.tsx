import "./globals.css";
import "./styles/home.css";
import "./styles/report.css";
import "./styles/docs-admin.css";
import TopBar from "@/components/TopBar";
import { SkipLink } from "@/components/LanguageMenu";
import Sidebar from "@/components/Sidebar";
import { I18nProvider } from "@/lib/i18n";
import type { Metadata, Viewport } from "next";

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  viewportFit: "cover",
};

export const metadata: Metadata = {
  metadataBase: new URL("https://chanshui.dev"),
  title: {
    default: "掺水？模型检测站｜中转站降智与接口审计",
    template: "%s｜掺水？模型检测站",
  },
  description:
    "掺水？模型检测站：面向模型中转站的降智检测、注入大小、缓存读写、工具调用、联网证据、推理表现审计与反作弊核验。",
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
      "检测模型中转站的注入大小、缓存链、工具调用、联网证据、推理表现与作假回放拦截。",
    locale: "zh_CN",
  },
  robots: { index: true, follow: true },
};

const PRE_PAINT_SCRIPT =
  "try{var d=document.documentElement,t=JSON.parse(localStorage.getItem('chanshui-theme')||'{}'),a=t.accent==='orange'?'orange':t.accent==='mint'?'mint':'mono',m=t.mode==='dark'?'dark':'light';d.dataset.accent=a;d.dataset.mode=m;d.style.colorScheme=m;var s=localStorage.getItem('chanshui-lang'),l=s==='zh'||s==='zh-Hant'||s==='en'||s==='ja'||s==='ko'?s:'';if(!l){var nav=navigator.languages&&navigator.languages.length?navigator.languages:[navigator.language||'zh'];for(var i=0;i<nav.length;i++){var n=String(nav[i]).toLowerCase();if(n.indexOf('zh')===0){l=/hant|tw|hk|mo/.test(n)?'zh-Hant':'zh';break}if(n.indexOf('en')===0){l='en';break}if(n.indexOf('ja')===0){l='ja';break}if(n.indexOf('ko')===0){l='ko';break}}if(!l)l='zh'}d.dataset.lang=l;d.lang=l==='zh'?'zh-CN':l}catch{}";

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="zh-CN" data-accent="mono" data-mode="light" suppressHydrationWarning>
      <head>
        <meta httpEquiv="Content-Type" content="text/html; charset=utf-8" />
        <script dangerouslySetInnerHTML={{ __html: PRE_PAINT_SCRIPT }} />
        <script
          dangerouslySetInnerHTML={{
            __html:
              "window.addEventListener('error',function(e){var t=e.target;if(!t||t.tagName!=='SCRIPT'||!t.src||t.src.indexOf('/_next/static/chunks/')<0)return;var k='chanshui-chunk-reload-at';var last=Number(sessionStorage.getItem(k))||0;if(Date.now()-last<60000)return;sessionStorage.setItem(k,String(Date.now()));location.reload()},true)",
          }}
        />
      </head>
      <body>
        <I18nProvider>
          <SkipLink />
          <Sidebar />
          <main className="main" id="main-content">
            <TopBar />
            <div className="main-stage">{children}</div>
          </main>
        </I18nProvider>
      </body>
    </html>
  );
}
