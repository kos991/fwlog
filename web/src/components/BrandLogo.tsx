import React from "react";
import { gsap } from "gsap";
import { MotionPathPlugin } from "gsap/MotionPathPlugin";

gsap.registerPlugin(MotionPathPlugin);

type BrandLogoProps = {
  className?: string;
  animated?: boolean;
};

export function BrandLogo({
  className = "",
  animated = false,
}: BrandLogoProps) {
  const root = React.useRef<HTMLSpanElement>(null);
  const shouldAnimate =
    animated || className.split(/\s+/).includes("brand-logo");
  React.useEffect(() => {
    if (
      !shouldAnimate ||
      !root.current ||
      window.matchMedia("(prefers-reduced-motion: reduce)").matches
    )
      return;
    const scope = root.current;
    const animations = ["top", "middle", "bottom"]
      .map((route, index) => {
        const particle = scope.querySelector(
          `[data-${"login-logo-particle"}][data-logo-route="${route}"]`,
        );
        const path = scope.querySelector(`#login-logo-route-${route}`);
        if (!particle || !path) return null;
        const svgPath = path as SVGPathElement;
        return gsap.to(particle, {
          motionPath: {
            path: svgPath,
            align: svgPath,
            autoRotate: false,
            alignOrigin: [0.5, 0.5],
          },
          keyframes: [
            { opacity: 0 },
            { opacity: 1 },
            { opacity: 1 },
            { opacity: 0 },
          ],
          duration: 2.8,
          delay: index * 0.35,
          ease: "none",
          repeat: -1,
        });
      })
      .filter(Boolean);
    const pulse = gsap.to(
      scope.querySelectorAll(
        `[data-${"login-logo-channel"}], .brand-mark__hub`,
      ),
      {
        opacity: 0.58,
        duration: 1.35,
        yoyo: true,
        repeat: -1,
        ease: "sine.inOut",
      },
    );
    return () => {
      animations.forEach((animation) => animation?.kill());
      pulse.kill();
    };
  }, [shouldAnimate]);

  return (
    <span
      ref={root}
      className={`brand-mark ${className}`.trim()}
      aria-hidden="true"
    >
      <svg
        className="brand-mark__graphic"
        viewBox="0 0 32 32"
        focusable="false"
      >
        <path
          id="login-logo-route-top"
          data-login-logo-channel
          className="brand-mark__channel"
          pathLength="1"
          d="M4 8h5c5 0 5 8 10 8h9"
        />
        <path
          id="login-logo-route-middle"
          data-login-logo-channel
          className="brand-mark__channel"
          pathLength="1"
          d="M4 16h24"
        />
        <path
          id="login-logo-route-bottom"
          data-login-logo-channel
          className="brand-mark__channel"
          pathLength="1"
          d="M4 24h5c5 0 5-8 10-8h9"
        />
        <circle className="brand-mark__hub" cx="19" cy="16" r="2.2" />
        <circle
          data-login-logo-particle
          data-logo-route="top"
          className="brand-mark__particle"
          r="1.35"
        />
        <circle
          data-login-logo-particle
          data-logo-route="middle"
          className="brand-mark__particle"
          r="1.35"
        />
        <circle
          data-login-logo-particle
          data-logo-route="bottom"
          className="brand-mark__particle"
          r="1.35"
        />
      </svg>
    </span>
  );
}
