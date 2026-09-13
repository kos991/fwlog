import React from 'react';
import { gsap } from 'gsap';
import { MotionPathPlugin } from 'gsap/MotionPathPlugin';
import { useGSAP } from '@gsap/react';

gsap.registerPlugin(useGSAP, MotionPathPlugin);

export type LoginSceneMotion = {
  ready: boolean;
  playSuccess: () => Promise<void>;
  playError: () => Promise<void>;
};

export function useLoginSceneMotion(root: React.RefObject<HTMLElement>): LoginSceneMotion {
  const [ready, setReady] = React.useState(false);
  const loopsRef = React.useRef<gsap.core.Animation[]>([]);
  const reducedMotionRef = React.useRef(false);

  const { contextSafe } = useGSAP(() => {
    if (!root.current) return;
    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    reducedMotionRef.current = reducedMotion;

    gsap.set('[data-login-title-char]', { opacity: 1, y: 8 });

    if (!reducedMotion) {
      const intro = gsap.timeline({ defaults: { ease: 'power2.out' } });
      intro
        .fromTo('[data-login-grid]', { opacity: 0 }, { opacity: 1, duration: 0.26 })
        .fromTo('[data-login-node]', { opacity: 0.72 }, { opacity: 1, duration: 0.42, stagger: 0.08 }, '-=0.12')
        .fromTo('[data-login-path], .login-scene-route-labels', { strokeDashoffset: 1 }, { strokeDashoffset: 0, duration: 0.48 }, '-=0.3')
        .fromTo('.login-panel', { y: 10 }, { y: 0, duration: 0.45 })
        .to('[data-login-title-char]', { opacity: 1, y: 0, duration: 0.28, stagger: 0.035 }, '-=0.25');

      const logoPulse = gsap.to('[data-login-logo-channel], .brand-mark__hub', { opacity: 0.58, duration: 1.2, yoyo: true, repeat: -1, ease: 'sine.inOut' });
      const logoParticles = ([
        ['top', '#login-logo-route-top', 0],
        ['middle', '#login-logo-route-middle', 0.32],
        ['bottom', '#login-logo-route-bottom', 0.64],
      ] as const).map(([route, path, delay]) => gsap.to(`[data-login-logo-particle][data-logo-route="${route}"]`, {
        motionPath: { path, align: path, autoRotate: false, alignOrigin: [0.5, 0.5] },
        keyframes: [{ opacity: 0 }, { opacity: 1 }, { opacity: 1 }, { opacity: 0 }],
        duration: 2.4, delay, ease: 'none', repeat: -1,
      }));
      const ingestMotion = { path: '#login-route-ingest', align: '#login-route-ingest', autoRotate: false, alignOrigin: [0.5, 0.5] };
      const archiveMotion = { path: '#login-route-archive', align: '#login-route-archive', autoRotate: false, alignOrigin: [0.5, 0.5] };
      const processParticles = [
        gsap.to('[data-login-particle][data-route="ingest"]', { motionPath: ingestMotion, keyframes: [{ opacity: 0.18 }, { opacity: 1 }, { opacity: 1 }, { opacity: 0.18 }], duration: 4.8, stagger: 1.35, ease: 'none', repeat: -1 }),
        gsap.to('[data-login-particle][data-route="archive"]', { motionPath: archiveMotion, keyframes: [{ opacity: 0.18 }, { opacity: 1 }, { opacity: 1 }, { opacity: 0.18 }], duration: 5.2, delay: 0.9, ease: 'none', repeat: -1 }),
        gsap.to('[data-login-node-icon] > *', { opacity: 0.58, strokeWidth: 2.6, duration: 1.2, yoyo: true, repeat: -1, stagger: 0.18, ease: 'sine.inOut' }),
        gsap.to('[data-login-progress]', { attr: { width: 224 }, duration: 2.8, yoyo: true, repeat: -1, ease: 'sine.inOut' }),
      ];
      loopsRef.current = [logoPulse, ...logoParticles, ...processParticles];
      if (document.hidden) loopsRef.current.forEach((animation) => animation.pause());
    } else {
      gsap.set('[data-login-grid], [data-login-node], [data-login-path], .login-scene-route-labels', { opacity: 1, strokeDashoffset: 0 });
      gsap.set('[data-login-progress]', { attr: { width: 150 } });
      gsap.set('[data-login-title-char]', { opacity: 1, y: 0 });
      gsap.set('.login-panel', { opacity: 1, y: 0 });
    }

    setReady(true);

    const handleVisibilityChange = () => loopsRef.current.forEach((animation) => (document.hidden ? animation.pause() : animation.resume()));
    document.addEventListener('visibilitychange', handleVisibilityChange);
    return () => {
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      loopsRef.current = [];
    };
  }, { scope: root });

  const playSuccess = contextSafe(() => new Promise<void>((resolve) => {
    if (reducedMotionRef.current) { resolve(); return; }
    loopsRef.current.forEach((animation) => animation.pause());
    gsap.timeline({ onComplete: resolve })
      .to('.login-panel', { scale: 1.012, duration: 0.18, yoyo: true, repeat: 1 })
      .to('.login-shell', { opacity: 0, duration: 0.28 });
  }));

  const playError = contextSafe(() => new Promise<void>((resolve) => {
    if (reducedMotionRef.current) { resolve(); return; }
    gsap.to('.login-panel', { x: 5, duration: 0.07, repeat: 5, yoyo: true, ease: 'power1.inOut', onComplete: resolve });
  }));

  return { ready, playSuccess, playError };
}
