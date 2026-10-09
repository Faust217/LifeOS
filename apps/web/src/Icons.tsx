import type { SVGProps } from "react";

import type { Area } from "./navigation";

type IconName = Area | "arrow" | "spark" | "shield" | "logout";

export function Icon({ name, ...props }: SVGProps<SVGSVGElement> & { name: IconName }) {
  const shared = {
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 1.8,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
  };

  return (
    <svg aria-hidden="true" focusable="false" viewBox="0 0 24 24" {...shared} {...props}>
      {name === "today" && (
        <>
          <circle cx="12" cy="12" r="3.5" />
          <path d="M12 2.5v3M12 18.5v3M2.5 12h3M18.5 12h3M5.3 5.3l2.2 2.2M16.5 16.5l2.2 2.2M18.7 5.3l-2.2 2.2M7.5 16.5l-2.2 2.2" />
        </>
      )}
      {name === "money" && (
        <>
          <rect x="3" y="5" width="18" height="14" rx="3" />
          <path d="M3 9h18M16 14h2" />
        </>
      )}
      {name === "plans" && (
        <>
          <rect x="4" y="3.5" width="16" height="17" rx="3" />
          <path d="m8 9 1.5 1.5L12 8M14.5 9.5H17M8 15h9" />
        </>
      )}
      {name === "me" && (
        <>
          <circle cx="12" cy="8" r="3.5" />
          <path d="M5 20c.7-3.2 3.2-5 7-5s6.3 1.8 7 5" />
        </>
      )}
      {name === "arrow" && <path d="M4 12h15m-6-6 6 6-6 6" />}
      {name === "spark" && <path d="m12 2 1.8 7.2L21 11l-7.2 1.8L12 20l-1.8-7.2L3 11l7.2-1.8L12 2Z" />}
      {name === "shield" && (
        <>
          <path d="M12 2.5 20 6v5.2c0 4.7-2.8 8-8 10.3-5.2-2.3-8-5.6-8-10.3V6l8-3.5Z" />
          <path d="m9 12 2 2 4-4" />
        </>
      )}
      {name === "logout" && (
        <>
          <path d="M10 4H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h4M15 7l5 5-5 5M9 12h11" />
        </>
      )}
    </svg>
  );
}
