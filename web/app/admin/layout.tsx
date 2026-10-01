import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "反馈记录",
  robots: { index: false, follow: false },
};

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return children;
}
