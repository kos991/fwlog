import * as THREE from './vendor/three.module.min.js';

const canvas = document.getElementById('login-globe-canvas');
const loginPanel = document.getElementById('login');
const reducedMotion = matchMedia('(prefers-reduced-motion: reduce)').matches;

function globePosition(longitude, latitude, radius = 1) {
  const lon = longitude * Math.PI / 180;
  const lat = latitude * Math.PI / 180;
  const cosLat = Math.cos(lat);
  return [radius * cosLat * Math.sin(lon), radius * Math.sin(lat), radius * cosLat * Math.cos(lon)];
}

function spherePoints(count, radius) {
  const positions = [];
  const goldenAngle = Math.PI * (3 - Math.sqrt(5));
  for (let index = 0; index < count; index += 1) {
    const y = 1 - (index / (count - 1)) * 2;
    const ringRadius = Math.sqrt(1 - y * y);
    const angle = goldenAngle * index;
    positions.push(Math.cos(angle) * ringRadius * radius, y * radius, Math.sin(angle) * ringRadius * radius);
  }
  return positions;
}

async function loadLandPoints() {
  const response = await fetch(new URL('./globe-land-points.json', import.meta.url));
  if (!response.ok) throw new Error(`地球点阵加载失败：HTTP ${response.status}`);
  return response.json();
}

async function mountGlobe() {
  if (!canvas || !loginPanel) return;

  const renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true, powerPreference: 'low-power' });
  renderer.setPixelRatio(Math.min(devicePixelRatio || 1, 1.5));
  renderer.setClearColor(0x000000, 0);
  renderer.setSize(112, 112, false);

  const scene = new THREE.Scene();
  const camera = new THREE.OrthographicCamera(-1.42, 1.42, 1.42, -1.42, 0.1, 10);
  camera.position.z = 4;
  const globe = new THREE.Group();
  globe.rotation.set(-0.14, -1.15, 0);
  scene.add(globe);

  const shellGeometry = new THREE.BufferGeometry();
  shellGeometry.setAttribute('position', new THREE.Float32BufferAttribute(spherePoints(280, 1), 3));
  globe.add(new THREE.Points(shellGeometry, new THREE.PointsMaterial({ color: 0x8a9bb2, size: 0.035, transparent: true, opacity: 0.2, depthWrite: false })));

  try {
    const landPoints = await loadLandPoints();
    const positions = landPoints.filter((_, index) => index % 2 === 0).flatMap(([longitude, latitude]) => globePosition(longitude, latitude, 1.018));
    const landGeometry = new THREE.BufferGeometry();
    landGeometry.setAttribute('position', new THREE.Float32BufferAttribute(positions, 3));
    globe.add(new THREE.Points(landGeometry, new THREE.PointsMaterial({ color: 0x4f73a9, size: 0.052, transparent: true, opacity: 0.8, depthWrite: false })));
  } catch {
    // 基础球体仍可显示，登录功能不依赖装饰资源。
  }

  const arcDefinitions = [
    [[121.47, 31.23], [13.4, 52.52], 0x2563eb, 0],
    [[114.17, 22.32], [139.69, 35.68], 0x0f766e, 0.48],
  ];
  const arcs = arcDefinitions.map(([from, to, color, offset]) => {
    const start = new THREE.Vector3(...globePosition(from[0], from[1], 1.025));
    const end = new THREE.Vector3(...globePosition(to[0], to[1], 1.025));
    const midpoint = start.clone().add(end).normalize().multiplyScalar(1.32);
    const curve = new THREE.QuadraticBezierCurve3(start, midpoint, end);
    const geometry = new THREE.BufferGeometry().setFromPoints(curve.getPoints(36));
    globe.add(new THREE.Line(geometry, new THREE.LineBasicMaterial({ color, transparent: true, opacity: 0.38, depthWrite: false })));
    const pulse = new THREE.Mesh(new THREE.SphereGeometry(0.04, 6, 6), new THREE.MeshBasicMaterial({ color }));
    globe.add(pulse);
    return { curve, pulse, offset };
  });

  let frame = 0;
  let elapsed = 0;
  let previousTime = performance.now();
  const shouldAnimate = () => !reducedMotion && !document.hidden && !loginPanel.hidden;
  const render = (time) => {
    frame = 0;
    const delta = Math.min(time - previousTime, 40) / 1000;
    previousTime = time;
    elapsed += delta;
    if (!reducedMotion) globe.rotation.y += delta * (Math.PI * 2 / 36);
    arcs.forEach(({ curve, pulse, offset }) => pulse.position.copy(curve.getPoint((elapsed * 0.16 + offset) % 1)));
    renderer.render(scene, camera);
    if (shouldAnimate()) frame = requestAnimationFrame(render);
  };
  const resume = () => {
    if (frame || !shouldAnimate()) return;
    previousTime = performance.now();
    frame = requestAnimationFrame(render);
  };

  const observer = new MutationObserver(resume);
  observer.observe(loginPanel, { attributes: true, attributeFilter: ['hidden'] });
  document.addEventListener('visibilitychange', resume);
  renderer.render(scene, camera);
  canvas.dataset.ready = 'true';
  canvas.closest('.login-globe')?.setAttribute('data-rendered', 'true');
  resume();
}

void mountGlobe();
