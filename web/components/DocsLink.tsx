"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import Icon from "@/components/Icon";
import { useI18n } from "@/lib/i18n";

export default function DocsLink() {
  const { t } = useI18n();
  const path = usePathname();
  const on = path === "/docs";
  return (
    <Link href="/docs" className={"theme-trigger docs-entry" + (on ? " is-on" : "")}>
      <Icon name="layers" width="16" height="16" />
      <span className="tool-label">{t("接口文档")}</span>
    </Link>
  );
}
