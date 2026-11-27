"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import GitHubLink from "@/components/GitHubLink";
import Icon, { IconName } from "@/components/Icon";
import LanguageMenu from "@/components/LanguageMenu";
import ThemeMenu from "@/components/ThemeMenu";
import { useI18n } from "@/lib/i18n";

const NAV_ITEMS: { href: string; label: string; icon: IconName }[] = [
  { href: "/", label: "新建检测", icon: "scan" },
  { href: "/history", label: "全站报告", icon: "fileText" },
  { href: "/docs", label: "接口文档", icon: "layers" },
];

export default function TopBar() {
  const { t } = useI18n();
  const pathname = usePathname() || "/";
  const isActive = (href: string) =>
    href === "/" ? pathname === "/" : pathname === href || pathname.startsWith(href + "/");

  return (
    <header className="app-topbar">
      <nav className="topbar-nav" aria-label={t("主导航")}>
        {NAV_ITEMS.map((item) => {
          const active = isActive(item.href);
          return (
            <Link
              key={item.href}
              href={item.href}
              className={"topbar-link" + (active ? " is-active" : "")}
              aria-current={active ? "page" : undefined}
            >
              <Icon name={item.icon} width="15" height="15" />
              <span>{t(item.label)}</span>
            </Link>
          );
        })}
      </nav>
      <div className="topbar-tools">
        <LanguageMenu />
        <ThemeMenu />
        <GitHubLink />
      </div>
    </header>
  );
}
