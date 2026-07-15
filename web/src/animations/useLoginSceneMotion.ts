import React from 'react';
import type { Scope } from 'animejs';

type AnimeModule = typeof import('animejs');
type PausableAnimation = {
  pause: () => unknown;
  resume: () => unknown;
};

export type LoginSceneMotion = {
  ready: boolean;
  playSuccess: () => Promise<void>;
  playError: () => Promise<void>;
};

function cleanupScope(scope: Scope | null) {
  if (scope) scope.revert();
}

export function useLoginSceneMotion(root: React.RefObject<HTMLElement>): LoginSceneMotion {
  const animeRef = React.useRef<AnimeModule | null>(null);
  const scopeRef = React.useRef<Scope | null>(null);
  const loopsRef = React.useRef<PausableAnimation[]>([]);
  const [ready, setReady] = React.useState(false);

  React.useEffect(() => {
    let disposed = false;

    const handleVisibilityChange = () => {
      for (const animation of loopsRef.current) {
        if (document.hidden) animation.pause();
        else animation.resume();
      }
    };

    document.addEventListener('visibilitychange', handleVisibilityChange);

    void import('animejs')
      .then((anime) => {
        if (disposed || !root.current) return;

        animeRef.current = anime;
        const scope = anime.createScope({
          root,
          mediaQueries: {
            mobile: '(max-width: 900px)',
            reducedMotion: '(prefers-reduced-motion: reduce)',
          },
        });

        scope.add((self) => {
          if (!self) return;
          loopsRef.current = [];

          if (self.matches.reducedMotion) return;

          anime.set('[data-login-logo-channel]', { strokeDashoffset: 1 });
          anime.set('[data-login-title-char]', { opacity: 0, translateY: 10 });

          anime
            .createTimeline({ defaults: { ease: 'out(3)' } })
            .add('[data-login-grid]', { opacity: [0, 1], duration: 260 })
            .add(
              '[data-login-node]',
              {
                opacity: [0, 1],
                delay: anime.stagger(80),
                duration: 420,
              },
              '-=120',
            )
            .add(
              '[data-login-path], .login-scene-route-labels',
              {
                opacity: [0, 1],
                strokeDashoffset: [1, 0],
                duration: 480,
              },
              '-=300',
            )
            .add(
              '.login-panel',
              {
                opacity: [0, 1],
                translateY: [12, 0],
                delay: anime.stagger(80),
                duration: 400,
              },
              '-=300',
            )
            .add('[data-login-logo-channel]', {
              strokeDashoffset: 0,
              delay: anime.stagger(70),
              duration: 420,
            }, '-=320')
            .add('[data-login-title-char]', {
              opacity: 1,
              translateY: 0,
              delay: anime.stagger(36),
              duration: 380,
            }, '-=260');

          const logoRoutes = [
            { name: 'top', path: '#login-logo-route-top', delay: 0 },
            { name: 'middle', path: '#login-logo-route-middle', delay: 320 },
            { name: 'bottom', path: '#login-logo-route-bottom', delay: 640 },
          ];

          loopsRef.current = logoRoutes.map((route) =>
            anime.animate(`[data-login-logo-particle][data-logo-route="${route.name}"]`, {
              ...anime.createMotionPath(route.path),
              opacity: [0, 1, 1, 0],
              duration: 2400,
              delay: route.delay,
              ease: 'linear',
              loop: true,
            }),
          );
          loopsRef.current.push(
            anime.animate('[data-login-logo-channel], .brand-mark__hub', {
              opacity: [0.58, 1, 0.58],
              duration: 2400,
              ease: 'inOut(2)',
              loop: true,
            }),
          );

          if (self.matches.mobile) return;

          const ingestMotion = anime.createMotionPath('#login-route-ingest');
          const archiveMotion = anime.createMotionPath('#login-route-archive');

          loopsRef.current.push(
            anime.animate('[data-login-node-icon] > *', {
              opacity: [0.58, 1, 0.58],
              strokeWidth: [1.8, 2.6, 1.8],
              duration: 2400,
              delay: anime.stagger(180),
              ease: 'inOut(2)',
              loop: true,
            }),
            anime.animate('[data-login-particle][data-route="ingest"]', {
              ...ingestMotion,
              opacity: [0, 1, 1, 0],
              duration: 5600,
              delay: anime.stagger(1200),
              ease: 'linear',
              loop: true,
            }),
            anime.animate('[data-login-particle][data-route="archive"]', {
              ...archiveMotion,
              opacity: [0, 0.86, 0.86, 0],
              duration: 6100,
              delay: 900,
              ease: 'linear',
              loop: true,
            }),
          );

          if (document.hidden) {
            loopsRef.current.forEach((animation) => animation.pause());
          }

          return () => {
            loopsRef.current = [];
          };
        });

        scopeRef.current = scope;
        setReady(true);
      })
      .catch((error: unknown) => {
        if (import.meta.env.DEV) {
          console.warn('登录页动效加载失败，已切换为静态模式。', error);
        }
        if (!disposed) setReady(false);
      });

    return () => {
      disposed = true;
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      cleanupScope(scopeRef.current);
      scopeRef.current = null;
      animeRef.current = null;
      loopsRef.current = [];
    };
  }, [root]);

  const playSuccess = React.useCallback(async () => {
    const anime = animeRef.current;
    const scope = scopeRef.current;
    if (!anime || !scope || scope.matches.reducedMotion) return;

    try {
      loopsRef.current.forEach((animation) => animation.pause());
      await scope.execute(() =>
        anime
          .createTimeline({ defaults: { ease: 'inOut(3)' } })
          .add('[data-login-logo-particle]', {
            opacity: [1, 0],
            scale: [1, 1.35, 0.4],
            duration: 240,
          })
          .add('[data-login-logo-channel], .brand-mark__hub', {
            opacity: [0.68, 1, 0.68],
            strokeWidth: [1.6, 2.4, 1.6],
            duration: 220,
          }, '-=180')
          .add('[data-login-title-char]', {
            opacity: [1, 0.82],
            translateY: [0, -2],
            scaleX: [1, 0.98],
            duration: 180,
          })
          .add('[data-login-particle]', { opacity: [1, 0], scale: [1, 0.35], duration: 170 }, '-=120')
          .add('.login-shell', { opacity: [1, 0], duration: 240 }, '-=70'),
      );
    } catch {
      return;
    }
  }, []);

  const playError = React.useCallback(async () => {
    const anime = animeRef.current;
    const scope = scopeRef.current;
    if (!anime || !scope || scope.matches.reducedMotion) return;

    try {
      await scope.execute(() =>
        anime.animate('.login-panel', {
          translateX: [0, -5, 5, -3, 3, 0],
          duration: 240,
          ease: 'inOut(3)',
        }),
      );
    } catch {
      return;
    }
  }, []);

  return { ready, playSuccess, playError };
}
