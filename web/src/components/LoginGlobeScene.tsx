import React from 'react';
import { gsap } from 'gsap';
import landPoints from '../assets/globe-land-points.json';
import {
  arcPoints,
  densifyLandPoints,
  globePosition,
  orbitRingPoints,
  orbitRingPosition,
  polylinePosition,
  spherePoints,
  type LandPoint,
} from './loginGlobeGeometry';

type ForeignLocation = {
  code: string;
  city: string;
  label: string;
  longitude: number;
  latitude: number;
};

type AccessRoute = {
  from: LandPoint;
  to: LandPoint;
  color: number;
  offset: number;
};

type ArcAnimation = {
  positions: number[];
  pulse: import('three').Mesh;
  offset: number;
};

type OrbitNode = {
  mesh: import('three').Mesh;
  radius: number;
  tilt: number;
  offset: number;
  speed: number;
};

// 弧线终点与轨道流动节点代表正在被访问的国外地点。
const foreignLocations: ForeignLocation[] = [
  { code: 'TYO', city: 'Tokyo', label: '东京', longitude: 139.69, latitude: 35.68 },
  { code: 'FRA', city: 'Frankfurt', label: '法兰克福', longitude: 8.68, latitude: 50.11 },
  { code: 'LAX', city: 'Los Angeles', label: '洛杉矶', longitude: -118.24, latitude: 34.05 },
  { code: 'SIN', city: 'Singapore', label: '新加坡', longitude: 103.82, latitude: 1.35 },
];

const accessRoutes: AccessRoute[] = [
  { from: [121.47, 31.23], to: [139.69, 35.68], color: 0x2563eb, offset: 0 },
  { from: [116.4, 39.9], to: [8.68, 50.11], color: 0x0f766e, offset: 0.28 },
  { from: [114.17, 22.32], to: [-118.24, 34.05], color: 0x3b6fb5, offset: 0.55 },
  { from: [113.26, 23.13], to: [103.82, 1.35], color: 0x1d7f74, offset: 0.78 },
];

/** 城市标签贴图。按设备像素比放大画布，避免缩放后文字发虚。 */
function createLocationLabel(THREE: typeof import('three'), location: ForeignLocation) {
  const scale = Math.min(window.devicePixelRatio || 1, 2) * 2;
  const width = 150;
  const height = 44;
  const labelCanvas = document.createElement('canvas');
  labelCanvas.width = Math.round(width * scale);
  labelCanvas.height = Math.round(height * scale);
  const context = labelCanvas.getContext('2d');
  if (!context) return null;
  context.scale(scale, scale);

  context.fillStyle = 'rgba(255,255,255,.94)';
  context.strokeStyle = 'rgba(37,99,235,.34)';
  context.lineWidth = 1;
  context.beginPath();
  context.roundRect(0.5, 0.5, width - 1, height - 1, 10);
  context.fill();
  context.stroke();

  context.fillStyle = '#274b7d';
  context.font = '600 17px "Segoe UI", "Microsoft YaHei", sans-serif';
  context.textAlign = 'left';
  context.textBaseline = 'middle';
  context.fillText(location.label, 13, height / 2);

  context.fillStyle = 'rgba(13,110,103,.92)';
  context.font = '600 13px "Segoe UI", sans-serif';
  context.textAlign = 'right';
  context.fillText(location.code, width - 13, height / 2 + 1);

  const texture = new THREE.CanvasTexture(labelCanvas);
  texture.colorSpace = THREE.SRGBColorSpace;
  texture.anisotropy = 4;
  const material = new THREE.SpriteMaterial({ map: texture, transparent: true, depthWrite: false, depthTest: false, opacity: 0.95 });
  const sprite = new THREE.Sprite(material);
  sprite.scale.set(0.6, 0.176, 1);
  sprite.userData.location = location.city;
  sprite.renderOrder = 6;
  return sprite;
}

export function LoginGlobeScene() {
  const canvasRef = React.useRef<HTMLCanvasElement>(null);

  React.useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    let disposed = false;
    let removeScene: (() => void) | undefined;

    void import('three').then((THREE) => {
      if (disposed) return;

      const renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true, powerPreference: 'low-power' });
      renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.5));
      renderer.setClearColor(0x000000, 0);

      const scene = new THREE.Scene();
      const camera = new THREE.OrthographicCamera(-1.46, 1.46, 1.46, -1.46, 0.1, 20);
      camera.position.set(0, 0, 6);

      // globe 自转，orbits 独立公转。
      const globe = new THREE.Group();
      globe.rotation.x = -0.14;
      globe.rotation.y = -1.15;
      scene.add(globe);
      const orbits = new THREE.Group();
      orbits.rotation.x = -0.14;
      scene.add(orbits);

      // 不透明内球写入深度，让背面的陆地点和弧线被真正遮住。
      // 这是地球呈现立体感的关键，缺了它点阵会前后透叠成一团线。
      // 用顶点色做极浅的明暗过渡，不上灯光，避免出现灰色金属球压掉蓝色点阵。
      const occluderGeometry = new THREE.SphereGeometry(0.997, 64, 48);
      const occluderPosition = occluderGeometry.getAttribute('position');
      const occluderColors = new Float32Array(occluderPosition.count * 3);
      const lightDirection = new THREE.Vector3(-0.42, 0.5, 0.76).normalize();
      const vertexNormal = new THREE.Vector3();
      for (let index = 0; index < occluderPosition.count; index += 1) {
        vertexNormal.set(occluderPosition.getX(index), occluderPosition.getY(index), occluderPosition.getZ(index)).normalize();
        const lambert = Math.max(0, vertexNormal.dot(lightDirection));
        const tone = 0.9 + lambert * 0.1;
        occluderColors[index * 3] = tone;
        occluderColors[index * 3 + 1] = tone * 0.995 + 0.005;
        occluderColors[index * 3 + 2] = Math.min(1, tone * 1.01);
      }
      occluderGeometry.setAttribute('color', new THREE.BufferAttribute(occluderColors, 3));
      const occluder = new THREE.Mesh(
        occluderGeometry,
        new THREE.MeshBasicMaterial({ vertexColors: true }),
      );
      occluder.renderOrder = 0;
      globe.add(occluder);

      const shellGeometry = new THREE.BufferGeometry();
      shellGeometry.setAttribute('position', new THREE.Float32BufferAttribute(spherePoints(1400, 1.004), 3));
      const shellMaterial = new THREE.PointsMaterial({ color: 0x8fa6c2, size: 0.009, transparent: true, opacity: 0.32, depthWrite: false });
      const shell = new THREE.Points(shellGeometry, shellMaterial);
      shell.renderOrder = 1;
      globe.add(shell);

      const densePoints = densifyLandPoints(landPoints as unknown as LandPoint[]);
      const landGeometry = new THREE.BufferGeometry();
      landGeometry.setAttribute(
        'position',
        new THREE.Float32BufferAttribute(densePoints.flatMap(([longitude, latitude]) => globePosition(longitude, latitude, 1.009)), 3),
      );
      const landMaterial = new THREE.PointsMaterial({ color: 0x2f6fc0, size: 0.0155, transparent: true, opacity: 1, depthWrite: false });
      const land = new THREE.Points(landGeometry, landMaterial);
      land.renderOrder = 2;
      globe.add(land);

      // 边缘光晕，只渲染背面让亮边留在球体轮廓上。
      const rim = new THREE.Mesh(
        new THREE.SphereGeometry(1.055, 48, 32),
        new THREE.MeshBasicMaterial({ color: 0x7ea6dd, transparent: true, opacity: 0.14, side: THREE.BackSide, depthWrite: false }),
      );
      globe.add(rim);

      const arcAnimations: ArcAnimation[] = [];
      accessRoutes.forEach(({ from, to, color, offset }) => {
        const positions = arcPoints(from, to, 84);
        const lineGeometry = new THREE.BufferGeometry();
        lineGeometry.setAttribute('position', new THREE.Float32BufferAttribute(positions, 3));
        const line = new THREE.Line(
          lineGeometry,
          new THREE.LineBasicMaterial({ color, transparent: true, opacity: 0.72, depthWrite: false }),
        );
        line.renderOrder = 3;
        globe.add(line);

        const pulse = new THREE.Mesh(
          new THREE.SphereGeometry(0.023, 12, 12),
          new THREE.MeshBasicMaterial({ color, transparent: true, opacity: 1, depthWrite: false }),
        );
        pulse.renderOrder = 4;
        globe.add(pulse);
        arcAnimations.push({ positions, pulse, offset });
      });

      const markers: Array<{
        marker: import('three').Mesh;
        halo: import('three').Mesh;
        label: import('three').Sprite | null;
        anchor: import('three').Vector3;
        labelAnchor: import('three').Vector3;
      }> = [];

      foreignLocations.forEach((location) => {
        const anchor = new THREE.Vector3(...globePosition(location.longitude, location.latitude, 1.014));
        const marker = new THREE.Mesh(
          new THREE.SphereGeometry(0.019, 12, 12),
          new THREE.MeshBasicMaterial({ color: 0x0f766e, transparent: true, depthWrite: false }),
        );
        marker.position.copy(anchor);
        marker.renderOrder = 5;
        globe.add(marker);

        const halo = new THREE.Mesh(
          new THREE.RingGeometry(0.032, 0.046, 24),
          new THREE.MeshBasicMaterial({ color: 0x14b8a6, transparent: true, opacity: 0.5, side: THREE.DoubleSide, depthWrite: false }),
        );
        halo.position.copy(anchor);
        halo.lookAt(0, 0, 0);
        halo.renderOrder = 5;
        globe.add(halo);

        const label = createLocationLabel(THREE, location);
        const labelAnchor = new THREE.Vector3(...globePosition(location.longitude, location.latitude, 1.1));
        if (label) {
          label.position.copy(labelAnchor);
          globe.add(label);
        }
        markers.push({ marker, halo, label, anchor, labelAnchor });
      });

      const orbitDefinitions = [
        { radius: 1.19, tilt: 18, speed: 0.055, nodes: 2, color: 0x2563eb, opacity: 0.46 },
        { radius: 1.28, tilt: -26, speed: -0.042, nodes: 2, color: 0x0f766e, opacity: 0.4 },
        { radius: 1.37, tilt: 8, speed: 0.031, nodes: 1, color: 0x4f7bbd, opacity: 0.32 },
      ];
      const orbitNodes: OrbitNode[] = [];
      orbitDefinitions.forEach(({ radius, tilt, speed, nodes, color, opacity }) => {
        const ringGeometry = new THREE.BufferGeometry();
        ringGeometry.setAttribute('position', new THREE.Float32BufferAttribute(orbitRingPoints(radius, tilt, 160), 3));
        orbits.add(new THREE.Line(ringGeometry, new THREE.LineBasicMaterial({ color, transparent: true, opacity, depthWrite: false })));

        for (let index = 0; index < nodes; index += 1) {
          const mesh = new THREE.Mesh(
            new THREE.SphereGeometry(0.015, 10, 10),
            new THREE.MeshBasicMaterial({ color, transparent: true, opacity: 0.85, depthWrite: false }),
          );
          orbits.add(mesh);
          orbitNodes.push({ mesh, radius, tilt, offset: index / nodes, speed });
        }
      });

      const resize = () => {
        const width = Math.max(1, canvas.clientWidth);
        const height = Math.max(1, canvas.clientHeight);
        renderer.setSize(width, height, false);
      };
      const resizeObserver = new ResizeObserver(resize);
      resizeObserver.observe(canvas);
      resize();

      const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
      let visible = !document.hidden;
      let elapsed = 0;
      // 每帧复用同一个向量，避免逐标记 clone 产生垃圾。
      const facingVector = new THREE.Vector3();

      const render = (_time: number, deltaTime: number) => {
        if (!visible) return;
        const delta = Math.min(deltaTime, 40) / 1000;
        elapsed += delta;
        if (!reducedMotion) {
          globe.rotation.y += delta * (Math.PI * 2 / 42);
          orbits.rotation.y += delta * 0.12;
        }

        arcAnimations.forEach(({ positions, pulse, offset }) => {
          pulse.position.set(...polylinePosition(positions, (elapsed * 0.14 + offset) % 1));
        });

        orbitNodes.forEach(({ mesh, radius, tilt, offset, speed }) => {
          mesh.position.set(...orbitRingPosition(radius, tilt, (elapsed * speed + offset + 1) % 1));
        });

        // 正面朝向摄像机的标记才显示，背面随自转淡出。
        // 标签的阈值明显高于圆点，保证文字只在球面正中区域出现，不会贴着边缘半透明地糊住。
        markers.forEach(({ marker, halo, label, anchor, labelAnchor }) => {
          const facing = facingVector.copy(anchor).applyQuaternion(globe.quaternion).z;
          const strength = Math.max(0, Math.min(1, (facing - 0.08) / 0.34));
          marker.visible = strength > 0;
          halo.visible = strength > 0;
          (marker.material as import('three').MeshBasicMaterial).opacity = 0.95 * strength;
          (halo.material as import('three').MeshBasicMaterial).opacity = 0.5 * strength;
          if (label) {
            const labelFacing = facingVector.copy(labelAnchor).applyQuaternion(globe.quaternion).z;
            const labelStrength = Math.max(0, Math.min(1, (labelFacing - 0.42) / 0.26));
            label.visible = labelStrength > 0.02;
            (label.material as import('three').SpriteMaterial).opacity = 0.95 * labelStrength;
          }
        });

        renderer.render(scene, camera);
      };

      const handleVisibilityChange = () => { visible = !document.hidden; };
      document.addEventListener('visibilitychange', handleVisibilityChange);
      render(0, 0);
      canvas.dataset.ready = 'true';
      if (!reducedMotion) gsap.ticker.add(render);

      removeScene = () => {
        gsap.ticker.remove(render);
        document.removeEventListener('visibilitychange', handleVisibilityChange);
        resizeObserver.disconnect();
        scene.traverse((object) => {
          if ('geometry' in object && object.geometry instanceof THREE.BufferGeometry) object.geometry.dispose();
          if ('material' in object) {
            const materials = Array.isArray(object.material) ? object.material : [object.material];
            materials.forEach((material) => {
              const map = (material as import('three').SpriteMaterial).map;
              if (map) map.dispose();
              material.dispose();
            });
          }
        });
        renderer.dispose();
      };
    });

    return () => {
      disposed = true;
      removeScene?.();
    };
  }, []);

  return (
    <div className="login-globe-scene" aria-hidden="true">
      <canvas ref={canvasRef} className="login-globe-scene__canvas" />
    </div>
  );
}
