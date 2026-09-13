export type LandPoint = readonly [number, number];

const DEGREE = Math.PI / 180;
const MASK_CELL_DEGREES = 3;

/** 经纬度转球面坐标，纬度为正朝上，经度 0 朝向摄像机正前方。 */
export function globePosition(longitude: number, latitude: number, radius = 1) {
  const lon = longitude * DEGREE;
  const lat = latitude * DEGREE;
  const cosLat = Math.cos(lat);
  return [
    radius * cosLat * Math.sin(lon),
    radius * Math.sin(lat),
    radius * cosLat * Math.cos(lon),
  ] as const;
}

function maskKey(lonCell: number, latCell: number) {
  return `${lonCell}:${latCell}`;
}

function buildLandMask(points: readonly LandPoint[]) {
  const mask = new Map<string, LandPoint[]>();
  points.forEach((point) => {
    const key = maskKey(Math.floor(point[0] / MASK_CELL_DEGREES), Math.floor(point[1] / MASK_CELL_DEGREES));
    const bucket = mask.get(key);
    if (bucket) bucket.push(point);
    else mask.set(key, [point]);
  });
  return mask;
}

/**
 * 用原始陆地点作为掩膜判断一个候选点是否落在陆地上。
 * 经度差按纬度收缩，避免高纬度把整圈都算成命中。
 */
function nearLand(mask: Map<string, LandPoint[]>, longitude: number, latitude: number, radiusDegrees: number) {
  const cosLat = Math.max(Math.cos(latitude * DEGREE), 0.08);
  const lonCell = Math.floor(longitude / MASK_CELL_DEGREES);
  const latCell = Math.floor(latitude / MASK_CELL_DEGREES);
  const reach = Math.ceil(radiusDegrees / cosLat / MASK_CELL_DEGREES);
  const limit = radiusDegrees * radiusDegrees;

  for (let lonOffset = -reach; lonOffset <= reach; lonOffset += 1) {
    for (let latOffset = -1; latOffset <= 1; latOffset += 1) {
      const bucket = mask.get(maskKey(lonCell + lonOffset, latCell + latOffset));
      if (!bucket) continue;
      for (const [candidateLon, candidateLat] of bucket) {
        let deltaLon = candidateLon - longitude;
        if (deltaLon > 180) deltaLon -= 360;
        if (deltaLon < -180) deltaLon += 360;
        const x = deltaLon * cosLat;
        const y = candidateLat - latitude;
        if (x * x + y * y <= limit) return true;
      }
    }
  }
  return false;
}

/**
 * 把原始 2.25° 陆地采样加密成等面积细网格。
 * 只用既有数据做掩膜判断，不引入新的地理数据文件。
 */
export function densifyLandPoints(
  points: readonly LandPoint[],
  spacingDegrees = 1.15,
  matchRadiusDegrees = 1.7,
): LandPoint[] {
  const mask = buildLandMask(points);
  const dense: LandPoint[] = [];
  for (let latitude = -84; latitude <= 84 + 1e-6; latitude += spacingDegrees) {
    const cosLat = Math.cos(latitude * DEGREE);
    if (cosLat <= 0.02) continue;
    const longitudeStep = spacingDegrees / cosLat;
    for (let longitude = -180; longitude < 180; longitude += longitudeStep) {
      if (nearLand(mask, longitude, latitude, matchRadiusDegrees)) {
        dense.push([longitude, latitude]);
      }
    }
  }
  return dense;
}

/** 均匀球面点阵，用于地球背景壳层。 */
export function spherePoints(count: number, radius: number) {
  const positions: number[] = [];
  const goldenAngle = Math.PI * (3 - Math.sqrt(5));
  for (let index = 0; index < count; index += 1) {
    const y = 1 - (index / (count - 1)) * 2;
    const ringRadius = Math.sqrt(1 - y * y);
    const angle = goldenAngle * index;
    positions.push(Math.cos(angle) * ringRadius * radius, y * radius, Math.sin(angle) * ringRadius * radius);
  }
  return positions;
}

/** 倾斜轨道环上的采样点，倾角单位为度。 */
export function orbitRingPoints(radius: number, tiltDegrees: number, segments = 128) {
  const tilt = tiltDegrees * DEGREE;
  const cosTilt = Math.cos(tilt);
  const sinTilt = Math.sin(tilt);
  const positions: number[] = [];
  for (let index = 0; index <= segments; index += 1) {
    const angle = (index / segments) * Math.PI * 2;
    const x = Math.cos(angle) * radius;
    const flat = Math.sin(angle) * radius;
    positions.push(x, flat * sinTilt, flat * cosTilt);
  }
  return positions;
}

/** 轨道环上给定进度的坐标，进度取 0 到 1。 */
export function orbitRingPosition(radius: number, tiltDegrees: number, progress: number) {
  const tilt = tiltDegrees * DEGREE;
  const angle = progress * Math.PI * 2;
  const flat = Math.sin(angle) * radius;
  return [Math.cos(angle) * radius, flat * Math.sin(tilt), flat * Math.cos(tilt)] as const;
}

/**
 * 大圆弧采样点。按两点夹角抬升弧线高度，保证整条弧始终在球面之外。
 * 二次贝塞尔在长距离会把控制点算到球面以内，导致弧线穿过地球，这里不使用。
 */
export function arcPoints(
  from: LandPoint,
  to: LandPoint,
  segments = 72,
  surfaceRadius = 1.012,
): number[] {
  const [ax, ay, az] = globePosition(from[0], from[1], 1);
  const [bx, by, bz] = globePosition(to[0], to[1], 1);
  const dot = Math.min(1, Math.max(-1, ax * bx + ay * by + az * bz));
  const omega = Math.acos(dot);
  const lift = 0.055 + (omega / Math.PI) * 0.42;
  const positions: number[] = [];

  for (let index = 0; index <= segments; index += 1) {
    const t = index / segments;
    let x: number;
    let y: number;
    let z: number;
    if (omega < 1e-6) {
      x = ax;
      y = ay;
      z = az;
    } else {
      const sinOmega = Math.sin(omega);
      const head = Math.sin((1 - t) * omega) / sinOmega;
      const tail = Math.sin(t * omega) / sinOmega;
      x = head * ax + tail * bx;
      y = head * ay + tail * by;
      z = head * az + tail * bz;
    }
    const length = Math.hypot(x, y, z) || 1;
    const radius = surfaceRadius + lift * Math.sin(Math.PI * t);
    positions.push((x / length) * radius, (y / length) * radius, (z / length) * radius);
  }
  return positions;
}

/** 在 arcPoints 生成的折线上按进度取点，用于流动光点。 */
export function polylinePosition(positions: readonly number[], progress: number) {
  const count = positions.length / 3;
  if (count === 0) return [0, 0, 0] as const;
  const clamped = Math.min(0.999999, Math.max(0, progress));
  const scaled = clamped * (count - 1);
  const index = Math.floor(scaled);
  const next = Math.min(count - 1, index + 1);
  const ratio = scaled - index;
  const base = index * 3;
  const head = next * 3;
  return [
    positions[base] + (positions[head] - positions[base]) * ratio,
    positions[base + 1] + (positions[head + 1] - positions[base + 1]) * ratio,
    positions[base + 2] + (positions[head + 2] - positions[base + 2]) * ratio,
  ] as const;
}
