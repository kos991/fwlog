import React from 'react';
import type { JSAnimation, Scope } from 'animejs';

type AnimeModule = typeof import('animejs');

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
  const loopsRef = React.useRef<JSAnimation[]>([]);
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

          anime
            .createTimeline({ defaults: { ease: 'out(3)' } })
            .add('[data-login-grid], .login-brand', { opacity: [0, 1], duration: 260 })
            .add(
              '[data-login-node]',
              {
                opacity: [0, 1],
                translateY: [10, 0],
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
              '.login-title-block, .login-panel',
              {
                opacity: [0, 1],
                translateY: [12, 0],
                delay: anime.stagger(80),
                duration: 400,
              },
              '-=300',
            );

          if (self.matches.mobile) return;

          const ingestMotion = anime.createMotionPath('#login-route-ingest');
          const archiveMotion = anime.createMotionPath('#login-route-archive');
          const queryMotion = anime.createMotionPath('#login-route-query');

          loopsRef.current = [
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
            anime.animate('[data-login-particle][data-route="query"]', {
              ...queryMotion,
              opacity: [0, 0.92, 0.92, 0],
              duration: 5200,
              delay: 1800,
              ease: 'linear',
              loop: true,
            }),
          ];

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
    if (!anime || !scope || scope.matches.mobile || scope.matches.reducedMotion) return;

    try {
      loopsRef.current.forEach((animation) => animation.pause());
      await scope.execute(() =>
        anime
          .createTimeline({ defaults: { ease: 'inOut(3)' } })
          .add('[data-login-particle]', { opacity: [1, 0], scale: [1, 0.35], duration: 170 })
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
