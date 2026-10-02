"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useI18n } from "@/lib/i18n";

export default function DocsLink() {
  const { t } = useI18n();
  const path = usePathname();
  const on = path === "/docs";
  return (
    <Link href="/docs" className={"theme-trigger docs-entry" + (on ? " is-on" : "")}>
      {t("接口文档")}
    </Link>
  );
}
