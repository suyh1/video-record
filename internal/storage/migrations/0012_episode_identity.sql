ALTER TABLE episodes ADD COLUMN absolute_number INTEGER;

UPDATE episodes
SET absolute_number = (
    SELECT COUNT(*)
    FROM episodes counted_episode
    JOIN seasons counted_season ON counted_season.id = counted_episode.season_id
    JOIN seasons target_season ON target_season.id = episodes.season_id
    WHERE counted_season.media_id = target_season.media_id
      AND (
          counted_season.season_number < target_season.season_number
          OR (
              counted_season.season_number = target_season.season_number
              AND counted_episode.episode_number <= episodes.episode_number
          )
      )
);
