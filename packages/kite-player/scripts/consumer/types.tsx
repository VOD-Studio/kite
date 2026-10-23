import type { AudioLike, DataSource, Lyrics, PlayerState, Track } from "@violetjs/kite-player";
import { Player } from "@violetjs/kite-player";
import { define, type KitePlayerElement } from "@violetjs/kite-player/element";
import {
  KitePlayer,
  KitePlaylist,
  KiteTrack,
  usePlaybackTime,
  usePlayer,
  usePlayerState,
} from "@violetjs/kite-player/react";
import { createRef } from "react";

// These declarations must resolve solely from the installed tarball, including its referenced chunks.
export function consume(source: DataSource, audio: AudioLike, track: Track, lyrics: Lyrics): PlayerState {
  const player = new Player({ source, audio, storageKey: false });
  player.setQueue([track], 0);
  player.seek(lyrics.lines[0]?.time ?? 0);
  define();
  const element: KitePlayerElement = document.createElement("kite-player");
  void element.open("lyrics");
  element.close();
  return player.state;
}

export function Consumer() {
  const ref = createRef<KitePlayerElement>();
  const player = usePlayer();
  const state = usePlayerState();
  const time: number = usePlaybackTime();
  return (
    <main data-playing={state.playing} data-time={time}>
      <button type="button" onClick={() => player.pause()}>
        Pause
      </button>
      <KitePlayer ref={ref} playlists="offline" collapse="none" theme="light" />
      <KiteTrack song="paper" size="l" />
      <KitePlaylist playlist="offline" size="m" />
    </main>
  );
}
