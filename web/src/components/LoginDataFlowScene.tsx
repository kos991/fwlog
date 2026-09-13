export function LoginDataFlowScene() {
  return (
    <div className="login-data-scene" aria-hidden="true">
      <svg
        className="login-data-scene__canvas"
        viewBox="0 0 1280 800"
        preserveAspectRatio="xMidYMin meet"
        shapeRendering="geometricPrecision"
        focusable="false"
      >
        <defs>
          <pattern id="login-grid-pattern" width="40" height="40" patternUnits="userSpaceOnUse">
            <path d="M40 0H0V40" fill="none" />
          </pattern>
          <filter id="login-particle-glow" x="-200%" y="-200%" width="500%" height="500%">
            <feGaussianBlur stdDeviation="3" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>

        <rect data-login-grid className="login-scene-grid" width="1280" height="800" fill="url(#login-grid-pattern)" />

        <g className="login-scene-links">
          <path
            data-login-path
            id="login-route-ingest"
            className="login-scene-path login-scene-path--main"
            pathLength="1"
            d="M180 406 C260 406 284 270 360 270 H520 C605 270 636 396 718 396 H884 C958 396 986 288 1072 288"
          />
          <path
            className="login-scene-path login-scene-path--glow"
            d="M180 406 C260 406 284 270 360 270 H520 C605 270 636 396 718 396 H884 C958 396 986 288 1072 288"
          />
          <path
            data-login-path
            id="login-route-archive"
            className="login-scene-path login-scene-path--branch"
            pathLength="1"
            d="M442 326 C442 442 548 520 718 520"
          />
          <path
            className="login-scene-path login-scene-path--glow login-scene-path--glow-branch"
            d="M442 326 C442 442 548 520 718 520"
          />
        </g>

        <g className="login-scene-route-labels">
          <text x="238" y="368">采集</text>
          <text x="592" y="314">入库</text>
          <text x="574" y="493">归档</text>
          <text x="962" y="330">检索</text>
        </g>

        <g className="login-scene-nodes">
          <g data-login-node className="login-scene-node" transform="translate(72 362)">
            <rect className="login-scene-node__surface" x="0" y="-14" width="150" height="106" rx="8" />
            <g data-login-node-icon className="login-scene-node__icon" transform="translate(32 0)">
              <rect width="30" height="28" rx="4" />
              <path d="M7 9h16M7 15h12M7 21h8" />
            </g>
            <text className="login-scene-node__title" x="32" y="56">日志源</text>
            <text className="login-scene-node__meta" x="32" y="77">文件目录</text>
          </g>

          <g data-login-node className="login-scene-node" transform="translate(360 214)">
            <rect className="login-scene-node__surface" x="0" y="-14" width="150" height="106" rx="8" />
            <g data-login-node-icon className="login-scene-node__icon" transform="translate(32 0)">
              <path d="M4 9h24v18H4zM10 4v5M22 4v5M10 18h12" />
            </g>
            <text className="login-scene-node__title" x="32" y="56">日志接收</text>
            <text className="login-scene-node__meta" x="32" y="77">网络日志</text>
            <circle className="login-scene-node__status" cx="134" cy="9" r="3" />
          </g>

          <g data-login-node className="login-scene-node" transform="translate(718 336)">
            <rect className="login-scene-node__surface" x="0" y="-14" width="150" height="106" rx="8" />
            <g data-login-node-icon className="login-scene-node__icon" transform="translate(32 0)">
              <ellipse cx="16" cy="7" rx="13" ry="5" />
              <path d="M3 7v17c0 3 6 5 13 5s13-2 13-5V7M3 15c0 3 6 5 13 5s13-2 13-5" />
            </g>
            <text className="login-scene-node__title" x="32" y="56">数据存储</text>
            <text className="login-scene-node__meta" x="32" y="77">日志数据</text>
          </g>

          <g data-login-node className="login-scene-node" transform="translate(1072 236)">
            <rect className="login-scene-node__surface" x="0" y="-14" width="150" height="106" rx="8" />
            <g data-login-node-icon className="login-scene-node__icon" transform="translate(32 0)">
              <circle cx="14" cy="14" r="10" />
              <path d="m22 22 8 8" />
            </g>
            <text className="login-scene-node__title" x="32" y="56">日志查询</text>
            <text className="login-scene-node__meta" x="32" y="77">快速检索</text>
          </g>

          <g data-login-node className="login-scene-node login-scene-node--archive" transform="translate(718 462)">
            <rect className="login-scene-node__surface" x="0" y="-14" width="150" height="106" rx="8" />
            <g data-login-node-icon className="login-scene-node__icon" transform="translate(32 0)">
              <path d="M4 10h24v18H4zM2 5h28v7H2zM12 17h8" />
            </g>
            <text className="login-scene-node__title" x="32" y="56">压缩归档</text>
            <text className="login-scene-node__meta" x="32" y="77">按策略保留</text>
          </g>
        </g>

        <g className="login-scene-particles" filter="url(#login-particle-glow)">
          <circle data-login-particle data-route="ingest" className="login-scene-particle" r="6" />
          <circle data-login-particle data-route="ingest" className="login-scene-particle" r="5" />
          <circle data-login-particle data-route="archive" className="login-scene-particle login-scene-particle--green" r="5" />
        </g>

        <g className="login-scene-progress" aria-hidden="true">
          <text className="login-scene-progress__title" x="72" y="690">日志处理流程</text>
          <rect className="login-scene-progress__rail" x="72" y="706" width="280" height="4" rx="2" />
          <rect data-login-progress className="login-scene-progress__value" x="72" y="706" width="150" height="4" rx="2" />
          <text className="login-scene-progress__meta" x="72" y="730">采集 · 接收 · 入库 · 查询</text>
        </g>
      </svg>
    </div>
  );
}
