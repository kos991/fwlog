type BrandLogoProps = {
  className?: string;
};

export function BrandLogo({ className = '' }: BrandLogoProps) {
  return (
    <span className={`brand-mark ${className}`.trim()} aria-hidden="true">
      <svg className="brand-mark__graphic" viewBox="0 0 32 32" focusable="false">
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
        <circle data-login-logo-particle data-logo-route="top" className="brand-mark__particle" r="1.35" />
        <circle data-login-logo-particle data-logo-route="middle" className="brand-mark__particle" r="1.35" />
        <circle data-login-logo-particle data-logo-route="bottom" className="brand-mark__particle" r="1.35" />
      </svg>
    </span>
  );
}
