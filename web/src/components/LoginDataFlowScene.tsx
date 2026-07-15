export function LoginDataFlowScene() {
  return (
    <div className="login-data-scene" aria-hidden="true">
      <svg
        className="login-data-scene__canvas"
        viewBox="0 0 1280 800"
        preserveAspectRatio="xMidYMin meet"
        focusable="false"
      >
        <defs>
          <pattern id="login-grid-pattern" width="40" height="40" patternUnits="userSpaceOnUse">
            <path d="M40 0H0V40" fill="none" />
          </pattern>
          <filter id="login-particle-glow" x="-200%" y="-200%" width="500%" height="500%">
            <feGaussianBlur stdDeviation="4" result="blur" />
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
            d="M204 410 C264 410 286 278 354 278 H520 C600 278 632 400 718 400 H884 C954 400 986 298 1070 298"
          />
          <path
            data-login-path
            id="login-route-archive"
            className="login-scene-path login-scene-path--branch"
            pathLength="1"
            d="M437 334 C437 448 548 526 718 526"
          />
        </g>

        <g className="login-scene-route-labels">
          <text x="236" y="374">采集</text>
          <text x="590" y="320">入库</text>
          <text x="566" y="500">归档</text>
          <text x="958" y="342">检索</text>
        </g>

        <g className="login-scene-nodes">
          <g data-login-node className="login-scene-node" transform="translate(62 354)">
            <rect className="login-scene-node__surface" width="142" height="112" rx="8" />
            <g data-login-node-icon className="login-scene-node__icon" transform="translate(18 18)">
              <rect width="32" height="30" rx="4" />
              <path d="M8 10h16M8 16h12M8 22h8" />
            </g>
            <text className="login-scene-node__title" x="18" y="72">日志源</text>
            <text className="login-scene-node__meta" x="18" y="94">文件目录</text>
          </g>

          <g data-login-node className="login-scene-node" transform="translate(354 222)">
            <rect className="login-scene-node__surface" width="166" height="112" rx="8" />
            <g data-login-node-icon className="login-scene-node__icon" transform="translate(18 18)">
              <path d="M4 9h24v18H4zM10 4v5M22 4v5M10 18h12" />
            </g>
            <text className="login-scene-node__title" x="18" y="72">日志接收</text>
            <text className="login-scene-node__meta" x="18" y="94">网络日志</text>
            <circle className="login-scene-node__status" cx="148" cy="20" r="4" />
          </g>

          <g data-login-node className="login-scene-node" transform="translate(718 344)">
            <rect className="login-scene-node__surface" width="166" height="112" rx="8" />
            <g data-login-node-icon className="login-scene-node__icon" transform="translate(18 18)">
              <ellipse cx="16" cy="7" rx="13" ry="5" />
              <path d="M3 7v17c0 3 6 5 13 5s13-2 13-5V7M3 15c0 3 6 5 13 5s13-2 13-5" />
            </g>
            <text className="login-scene-node__title" x="18" y="72">数据存储</text>
            <text className="login-scene-node__meta" x="18" y="94">日志数据</text>
          </g>

          <g data-login-node className="login-scene-node" transform="translate(1070 242)">
            <rect className="login-scene-node__surface" width="154" height="112" rx="8" />
            <g data-login-node-icon className="login-scene-node__icon" transform="translate(18 18)">
              <circle cx="14" cy="14" r="10" />
              <path d="m22 22 8 8" />
            </g>
            <text className="login-scene-node__title" x="18" y="72">日志查询</text>
            <text className="login-scene-node__meta" x="18" y="94">快速检索</text>
          </g>

          <g data-login-node className="login-scene-node login-scene-node--archive" transform="translate(718 470)">
            <rect className="login-scene-node__surface" width="166" height="112" rx="8" />
            <g data-login-node-icon className="login-scene-node__icon" transform="translate(18 18)">
              <path d="M4 10h24v18H4zM2 5h28v7H2zM12 17h8" />
            </g>
            <text className="login-scene-node__title" x="18" y="72">压缩归档</text>
            <text className="login-scene-node__meta" x="18" y="94">按策略保留</text>
          </g>
        </g>

        <g className="login-scene-particles" filter="url(#login-particle-glow)">
          <circle data-login-particle data-route="ingest" className="login-scene-particle" r="5" />
          <circle data-login-particle data-route="ingest" className="login-scene-particle" r="4" />
          <circle data-login-particle data-route="archive" className="login-scene-particle login-scene-particle--green" r="4" />
        </g>

        <g className="login-scene-telemetry" transform="translate(78 642)">
          <text className="login-scene-telemetry__label" x="0" y="0">日志处理流程</text>
          <rect className="login-scene-telemetry__rail" x="0" y="18" width="316" height="4" rx="2" />
          <rect className="login-scene-telemetry__value" x="0" y="18" width="196" height="4" rx="2" />
          <text className="login-scene-telemetry__meta" x="0" y="48">采集 · 接收 · 入库 · 查询</text>
        </g>
      </svg>
    </div>
  );
}
