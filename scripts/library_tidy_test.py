"""Tests for the library-tidy planner.

    python3 -m unittest discover -s scripts -p 'library_tidy_test.py'

The planner is the whole risk of this tool: it decides what a folder holding
someone's media gets called. Every case below is a shape that actually turns up
in a hand-built library.
"""
import unittest

from library_tidy import build_plan, extract_year, plan_loose_files, target_name


class ExtractYear(unittest.TestCase):
    def test_takes_the_trailing_release_year(self):
        _, y = extract_year("Dune 2021")
        self.assertEqual(y, 2021)

    def test_prefers_the_last_year_when_the_title_contains_one(self):
        # "Blade Runner 2049" was released in 2017. Taking the FIRST year would
        # rename it to "Blade Runner (2049)" and lose the real one.
        rest, y = extract_year("Blade Runner 2049 2017")
        self.assertEqual(y, 2017)
        self.assertIn("2049", rest)

    def test_keeps_a_year_that_is_the_whole_title(self):
        # "1917" and "2012" are films. Stripping the year leaves nothing.
        self.assertEqual(extract_year("1917"), ("1917", None))
        self.assertEqual(extract_year("2012"), ("2012", None))

    def test_no_year_at_all(self):
        self.assertEqual(extract_year("Friends"), ("Friends", None))

    def test_ignores_longer_digit_runs(self):
        _, y = extract_year("Movie 12345")
        self.assertIsNone(y)


class MovieNames(unittest.TestCase):
    def check(self, raw, want):
        self.assertEqual(target_name(raw, is_tv=False), want)

    def test_synodl_own_shape_gains_parentheses(self):
        # This is the systematic change: SynoDL writes "Title Year".
        self.check("Despicable Me 4 2024", "Despicable Me 4 (2024)")

    def test_already_correct_is_left_alone(self):
        self.check("Dune (2021)", "Dune (2021)")

    def test_scene_release(self):
        self.check("Dune.2021.1080p.BluRay.x264-RARBG", "Dune (2021)")
        self.check("The.Matrix.1999.2160p.UHD.BluRay.x265-TERMiNAL", "The Matrix (1999)")

    def test_bracketed_noise(self):
        self.check("Arrival (2016) [1080p] [BluRay]", "Arrival (2016)")
        self.check("Interstellar 2014 {Farsi Dubbed}", "Interstellar (2014)")

    def test_underscores_and_dots(self):
        self.check("Blade_Runner_2049_2017_REMUX", "Blade Runner 2049 (2017)")

    def test_recases_only_unambiguous_lowercase(self):
        # A multi-word lowercase name is unambiguously un-cased text.
        self.check("the matrix 1999", "The Matrix (1999)")
        self.check("the dark knight 2008", "The Dark Knight (2008)")
        # Each alphabetic run, so a hyphenated title reads properly.
        self.check("spider-man.no.way.home.2021.extended", "Spider-Man No Way Home (2021)")
        # ALL CAPS is left alone: "WALL-E" and "UP" are titles, not shouting, and
        # scrapers match case-insensitively — so re-casing risks a wrong rename
        # to fix nothing.
        self.check("WALL-E 2008", "WALL-E (2008)")
        self.check("THE MATRIX 1999", "THE MATRIX (1999)")
        # A single lowercase word may be exactly how the title looks.
        self.check("iCarly 2007", "iCarly (2007)")

    def test_title_that_is_only_a_year_keeps_it(self):
        self.check("1917", "1917")
        self.check("1917.2019.1080p.BluRay", "1917 (2019)")

    def test_junk_word_inside_a_title_is_not_eaten(self):
        # "Her" must not be shortened by the "hdr" token, and a real word that
        # merely contains a junk substring has to survive.
        self.check("Her 2013", "Her (2013)")
        self.check("Cameraperson 2016", "Cameraperson (2016)")
        self.check("Tomorrowland 2015", "Tomorrowland (2015)")

    def test_unparseable_name_is_left_alone_rather_than_mangled(self):
        self.assertEqual(target_name("1080p", is_tv=False), "1080p")


class TvNames(unittest.TestCase):
    def check(self, raw, want):
        self.assertEqual(target_name(raw, is_tv=True), want)

    def test_season_spans_are_dropped(self):
        self.check("Friends S01-S10 COMPLETE 1994", "Friends (1994)")
        self.check("Breaking Bad Season 1-5 2008", "Breaking Bad (2008)")
        self.check("The Wire S01 2002", "The Wire (2002)")

    def test_synodl_year_range_keeps_the_first_year(self):
        # SynoDL writes the source's range, e.g. "Friends 1994 - 2004". Plex
        # wants the FIRST air year, so the range must not survive.
        self.assertEqual(target_name("Friends 1994 - 2004", is_tv=True), "Friends (1994)")

    def test_ongoing_series_open_range(self):
        self.assertEqual(target_name("Severance 2022 -", is_tv=True), "Severance (2022)")


class PlanSafety(unittest.TestCase):
    def test_refuses_to_rename_onto_an_existing_folder(self):
        # Merging two folders is exactly the outcome that loses files.
        plan = build_plan("/movie", ["Dune 2021", "Dune (2021)"], is_tv=False)
        actions = {c.old: c.action for c in plan.changes}
        self.assertEqual(actions["Dune 2021"], "conflict")
        self.assertEqual(actions["Dune (2021)"], "keep")
        self.assertEqual(plan.renames, [])

    def test_refuses_when_two_folders_want_the_same_name(self):
        plan = build_plan("/movie", ["Dune.2021.1080p", "Dune.2021.2160p"], is_tv=False)
        self.assertEqual(len(plan.renames), 1)
        self.assertEqual(len(plan.problems), 1)

    def test_correct_folders_are_not_touched(self):
        plan = build_plan("/movie", ["Arrival (2016)", "Dune (2021)"], is_tv=False)
        self.assertEqual(plan.renames, [])
        self.assertEqual(plan.problems, [])

    def test_never_proposes_a_path_separator(self):
        # A proposed name containing a separator would be a traversal bug, not a
        # rename. DSM will not produce such a folder name, so this guards the
        # planner rather than a real listing.
        for name in ["A/B 2020", "A\\B 2020"]:
            plan = build_plan("/movie", [name], is_tv=False)
            for c in plan.renames:
                self.assertNotIn("/", c.new)
                self.assertNotIn("\\", c.new)


if __name__ == "__main__":
    unittest.main()


class LooseFileGrouping(unittest.TestCase):
    """Files dumped straight into a parent, with their sidecars."""

    def test_groups_a_movie_with_its_subtitles_and_artwork(self):
        moves, loose = plan_loose_files(
            [
                "Dune.2021.1080p.BluRay.x264.mkv",
                "Dune.2021.1080p.BluRay.x264.srt",
                "Dune.2021.1080p.BluRay.x264.en.srt",
                "Dune.2021.1080p.BluRay.x264.nfo",
                "Dune.2021.1080p.BluRay.x264-poster.jpg",
            ],
            existing_dirs=[],
            is_tv=False,
        )
        self.assertEqual(len(moves), 1)
        self.assertEqual(moves[0].dest, "Dune (2021)")
        self.assertTrue(moves[0].create)
        self.assertEqual(len(moves[0].files), 5, f"sidecars left behind: {moves[0].files}")
        self.assertEqual(loose, [])

    def test_two_different_films_do_not_merge(self):
        moves, _ = plan_loose_files(
            ["Dune.2021.mkv", "Dune.2021.srt", "Arrival.2016.mkv"],
            existing_dirs=[], is_tv=False,
        )
        self.assertEqual({m.dest for m in moves}, {"Dune (2021)", "Arrival (2016)"})

    def test_moves_into_an_existing_folder_rather_than_a_near_duplicate(self):
        # The folder is already correct; a loose extra must land IN it, not beside
        # it in a second folder with almost the same name.
        moves, _ = plan_loose_files(
            ["Dune.2021.1080p.mkv"], existing_dirs=["Dune (2021)"], is_tv=False,
        )
        self.assertEqual(len(moves), 1)
        self.assertEqual(moves[0].dest, "Dune (2021)")
        self.assertFalse(moves[0].create)

    def test_matches_an_existing_folder_written_differently(self):
        moves, _ = plan_loose_files(
            ["Dune.2021.1080p.mkv"], existing_dirs=["Dune 2021"], is_tv=False,
        )
        self.assertEqual(moves[0].dest, "Dune 2021")
        self.assertFalse(moves[0].create)

    def test_a_video_is_required_to_anchor_a_group(self):
        # A stray subtitle with no film present must NOT invent a folder — we
        # have no idea whether the film is elsewhere or missing entirely.
        moves, loose = plan_loose_files(
            ["Some.Film.2019.srt"], existing_dirs=[], is_tv=False,
        )
        self.assertEqual(moves, [])
        self.assertEqual([n for n, _ in loose], ["Some.Film.2019.srt"])

    def test_generic_sidecars_are_never_attributed(self):
        # "poster.jpg" belongs to nothing in particular. Guessing would file a
        # stranger's artwork inside somebody's film.
        moves, loose = plan_loose_files(
            ["Dune.2021.mkv", "poster.jpg", "folder.jpg", "movie.nfo", "cover.png"],
            existing_dirs=[], is_tv=False,
        )
        self.assertEqual(len(moves), 1)
        self.assertEqual(moves[0].files, ["Dune.2021.mkv"])
        self.assertEqual(
            sorted(n for n, _ in loose), ["cover.png", "folder.jpg", "movie.nfo", "poster.jpg"]
        )

    def test_unparseable_file_is_left_alone(self):
        moves, loose = plan_loose_files(["VID_20190104_120000.mp4"], existing_dirs=[], is_tv=False)
        self.assertEqual(moves, [])
        self.assertEqual(len(loose), 1)

    def test_tv_episodes_go_into_season_folders(self):
        moves, _ = plan_loose_files(
            [
                "Friends.S01E01.1080p.mkv",
                "Friends.S01E01.1080p.srt",
                "Friends.S01E02.1080p.mkv",
                "Friends.S02E01.1080p.mkv",
            ],
            existing_dirs=["Friends (1994)"],
            is_tv=True,
        )
        by_season = {(m.dest, m.season): sorted(m.files) for m in moves}
        self.assertEqual(
            by_season,
            {
                ("Friends (1994)", "Season 01"): [
                    "Friends.S01E01.1080p.mkv", "Friends.S01E01.1080p.srt", "Friends.S01E02.1080p.mkv",
                ],
                ("Friends (1994)", "Season 02"): ["Friends.S02E01.1080p.mkv"],
            },
        )

    def test_tv_episode_with_no_show_folder_creates_one(self):
        moves, _ = plan_loose_files(["The.Bear.S01E01.mkv"], existing_dirs=[], is_tv=True)
        self.assertEqual(moves[0].dest, "The Bear")
        self.assertTrue(moves[0].create)
        self.assertEqual(moves[0].season, "Season 01")

    def test_tv_file_without_a_season_lands_in_the_show_folder(self):
        moves, _ = plan_loose_files(
            ["Chernobyl.2019.1080p.mkv"], existing_dirs=["Chernobyl (2019)"], is_tv=True,
        )
        self.assertEqual(moves[0].dest, "Chernobyl (2019)")
        self.assertIsNone(moves[0].season)

    def test_never_proposes_moving_onto_an_existing_name(self):
        # Two copies of the same episode at different qualities would collide
        # once both are moved; the planner must not silently pick a winner.
        moves, loose = plan_loose_files(
            ["Friends.S01E01.720p.mkv", "Friends.S01E01.1080p.mkv"],
            existing_dirs=[], is_tv=True,
        )
        moved = sum(len(m.files) for m in moves)
        self.assertEqual(moved + len(loose), 2)
        for m in moves:
            self.assertEqual(len(set(m.files)), len(m.files))


class RealLibraryRegressions(unittest.TestCase):
    """Cases taken from a real library's dry run, where the planner got it wrong.

    Each one was a rename that would have been applied. The first is the worst:
    a film reduced to the word "The".
    """

    def m(self, raw, want):
        self.assertEqual(target_name(raw, is_tv=False), want)

    def t(self, raw, want):
        self.assertEqual(target_name(raw, is_tv=True), want)

    def test_a_language_word_inside_a_title_is_not_a_release_tag(self):
        # "Persian" was treated as a dubbing tag, truncating the title to "The".
        self.m("The Persian Version 2023", "The Persian Version (2023)")
        self.m("My_Favorite_Cake_Irani", "My Favorite Cake Irani")

    def test_a_subtitle_after_a_dash_is_not_a_release_group(self):
        # "- Prodigy" looked like "-RARBG" and was stripped, which would have
        # merged a spin-off into the parent show's folder.
        self.t("Star Trek - Prodigy 2021", "Star Trek - Prodigy (2021)")
        self.m("Hotel Transylvania - Transformania 2022", "Hotel Transylvania - Transformania (2022)")
        self.m("Birth-Rebirth 2023", "Birth-Rebirth (2023)")
        self.m("Primal - Tales of Savagery 2019", "Primal - Tales of Savagery (2019)")

    def test_words_that_are_release_tags_only_after_the_year(self):
        self.m("Grand Theft Auto VI An Extended Look 2026", "Grand Theft Auto VI An Extended Look (2026)")
        self.m("A.Real.Pain.2024.Hybrid.2160p.WEB-DL.DV.HDR.DDP5.1.Atmos.H265-AOC", "A Real Pain (2024)")
        self.m("Companion.2025.Hybrid.2160p.WEB-DL.DV.HDR.DDP5.1.Atmos.H265-AOC", "Companion (2025)")

    def test_a_language_after_the_year_is_dropped(self):
        self.m("Kill.Boksoon.2023.KOREAN.1080p.NF.WEB-DL.x265.10bit.HDR.DDP5.Atmos-WDYM", "Kill Boksoon (2023)")
        self.m("JUNGE.2023.KOREAN.1080p.NF.WEBRip.DDP5.1.Atmos.x264-SMURF", "JUNGE (2023)")
        self.m("Ride.On.2023.CHINESE.1080p.WEBRip.1400MB.DD5.1.x264-GalaxyRG[TGx]", "Ride On (2023)")
        self.m("Project.Wolf.Hunting.2022.KOREAN.1080p.WEBRip.x265-VXT", "Project Wolf Hunting (2022)")

    def test_initials_keep_their_dots(self):
        # Dots are a scene separator, but "E.T." and "M.D." are how the titles
        # are spelled. Only convert when the name is dot-separated throughout.
        self.m("E.T. the Extra-Terrestrial 1982", "E.T. the Extra-Terrestrial (1982)")
        self.t("House M.D. 2004", "House M.D. (2004)")
        self.t("Mr. Robot 2015", "Mr. Robot (2015)")
        self.m("Fantastic Mr. Fox 2009", "Fantastic Mr. Fox (2009)")
        self.m("Megan 2.0", "Megan 2.0")
        # ...but a genuinely dot-separated scene name still converts.
        self.m("Society.of.the.Snow.2023.4K.SDR.2160p.WEBDL Ita Eng x265-NAHOM", "Society of the Snow (2023)")
        self.m("Dune.Part.Two.2024.1080p.WEB-DL.DDP2.0.H264-AOC", "Dune Part Two (2024)")

    def test_a_year_inside_the_title_survives(self):
        self.m("Hijack.1971.2024.1080p.WEB-DL.DD5.1.H.264-FLUX[TGx]", "Hijack 1971 (2024)")
        self.m("1992.2024.1080p.AMZN.WEBRip.1400MB.DD5.1.x264-GalaxyRG[TGx]", "1992 (2024)")
        self.m("65.2023.2160p.WEB-DL.x265.10bit.HDR.DDP5.1-FLUX", "65 (2023)")
        self.m("Together 99 (2023) [1080p] [WEBRip] [5.1] [YTS.MX]", "Together 99 (2023)")

    def test_trailing_separator_before_the_year(self):
        self.t("Breaking Bad - 2008", "Breaking Bad (2008)")
        self.m("Exhuma - 2024", "Exhuma (2024)")
        self.m("Woman and Child - 2025", "Woman and Child (2025)")

    def test_series_pack_names_without_a_year(self):
        self.t("Breaking.Bad.S01-S05.COMPLETE.1080p.BluRay-GROUP", "Breaking Bad")
        self.m("THE.HUNGER.GAMES.THE.BALLAD.OF.SONGBIRDS.AND.SNAKES.1080p.WEB.DL-MassModz",
               "THE HUNGER GAMES THE BALLAD OF SONGBIRDS AND SNAKES")

    def test_titles_that_were_already_right_stay_right(self):
        for raw, want in [
            ("Dune.2021.1080p.BluRay.x264-RARBG", "Dune (2021)"),
            ("A Simple Favor (2018) [2160p] [4K] [BluRay] [5.1] [YTS.MX]", "A Simple Favor (2018)"),
            ("Vicky Cristina Barcelona (2008) (1080p BluRay x265 HEVC 10bit AAC 3.0 afm72)",
             "Vicky Cristina Barcelona (2008)"),
            ("Kandahar (2023) (2160p MA WEB-DL H265 HDR DDP 5.1 English - HONE)[TGx]", "Kandahar (2023)"),
            ("Don 2 2011 1080p BluRay x265 Hindi DDP5.1 - SP3LL", "Don 2 (2011)"),
            ("Chloe_2009_1080p_BluRay_REMUX", "Chloe (2009)"),
            ("Whiplash 2014 1080p HQ BluRay AV1 Opus 5.1 [981]", "Whiplash (2014)"),
            ("Justice League – Gotham City Breakout 2016", "Justice League – Gotham City Breakout (2016)"),
            ("Léon The Professional 1994", "Léon The Professional (1994)"),
        ]:
            self.assertEqual(target_name(raw, is_tv=False), want, raw)


class DuplicateFolders(unittest.TestCase):
    """Two folders for one title. The tool must say so, not pick one silently."""

    def test_reports_two_folders_for_the_same_title(self):
        plan = build_plan("/movie", ["Hoppers", "Hoppers 2026"], is_tv=False)
        self.assertTrue(plan.duplicates, "two folders for one title were not reported")
        names = {n for group in plan.duplicates for n in group}
        self.assertEqual(names, {"Hoppers", "Hoppers 2026"})

    def test_different_years_are_not_duplicates(self):
        plan = build_plan("/movie", ["Nefarious 2019", "Nefarious 2023"], is_tv=False)
        self.assertEqual(plan.duplicates, [])

    def test_loose_files_prefer_the_folder_whose_year_matches(self):
        moves, _ = plan_loose_files(
            ["Hoppers_2026_2160p_x265.mkv"],
            existing_dirs=["Hoppers", "Hoppers (2026)"], is_tv=False,
        )
        self.assertEqual(moves[0].dest, "Hoppers (2026)")


class TitleCasing(unittest.TestCase):
    """Capitalise words that are entirely lowercase; never touch anything else.

    The safety of this rests on one observation: a stylised title always carries
    an uppercase letter somewhere ("iCarly", "M3GAN", "MaXXXine", "EXmas", "IF"),
    so a rule that only ever touches an all-lowercase word cannot reach them.
    """

    def m(self, raw, want):
        self.assertEqual(target_name(raw, is_tv=False), want)

    def test_a_single_lowercase_word(self):
        self.m("cuckoo 2024", "Cuckoo (2024)")
        self.m("constantine 2005", "Constantine (2005)")
        self.m("playdate 2025", "Playdate (2025)")
        self.m("nobody 2", "Nobody 2")

    def test_lowercase_words_inside_a_partly_cased_title(self):
        self.m("meet Joe Black 1998", "Meet Joe Black (1998)")
        self.m("Free guy 2020", "Free Guy (2020)")
        self.m("Fantastic four 2025", "Fantastic Four (2025)")
        self.m("The naked gun 2025", "The Naked Gun (2025)")
        self.m("Devil wears Prada 2 2026", "Devil Wears Prada 2 (2026)")
        self.m("Sentimental value 2025", "Sentimental Value (2025)")

    def test_small_words_stay_lowercase_unless_first_or_last(self):
        self.m("By the sea 2015", "By the Sea (2015)")
        self.m("In the fade 2017", "In the Fade (2017)")
        self.m("Talk to me 2022", "Talk to Me (2022)")
        self.m("Masters of the universe", "Masters of the Universe")
        self.m("in the grey", "In the Grey")
        # A small word IS capitalised when it opens or closes the title.
        self.m("the drama", "The Drama")
        self.m("Tell no one 2006", "Tell No One (2006)")

    def test_a_word_carrying_any_uppercase_is_left_alone(self):
        # These are the titles the rule exists to protect.
        for raw in ["MaXXXine 2024", "M3GAN 2023", "EXmas 2023", "IF 2024", "PK 2014",
                    "JUNGE 2023", "Expend4bles 2023", "iCarly 2007", "WALL-E 2008"]:
            expected = raw.rsplit(" ", 1)[0] + " (" + raw.rsplit(" ", 1)[1] + ")"
            self.assertEqual(target_name(raw, is_tv=False), expected, raw)
        self.m("THE.HUNGER.GAMES.THE.BALLAD.OF.SONGBIRDS.AND.SNAKES.1080p.WEB.DL-MassModz",
               "THE HUNGER GAMES THE BALLAD OF SONGBIRDS AND SNAKES")

    def test_accented_and_hyphenated_words(self):
        # A naive [a-z] capitaliser turns "colère" into "ColèRe".
        self.m("Pattie et la colère de Poséidon 2022", "Pattie et la Colère de Poséidon (2022)")
        self.m("spider-man.no.way.home.2021.extended", "Spider-Man No Way Home (2021)")

    def test_non_english_particles_stay_lowercase(self):
        self.m("Operation.Fortune.Ruse.de.Guerre.2023.1080p.WEBRip.DDP5.1.x264-RiGHTNOW",
               "Operation Fortune Ruse de Guerre (2023)")

    def test_a_lowercase_x_is_the_versus_sense(self):
        self.m("Godzilla.x.Kong.The.New.Empire.2024.2160p.WEB-DL.DDP5.1.HDR10+.H.265-POKE[TGx]",
               "Godzilla x Kong The New Empire (2024)")
        # An uppercase X in the source is part of the title and stays.
        self.m("Saw.X.2023.2160p.WEB-DL.DDP5.1.Atmos.H.265-FLUX[TGx]", "Saw X (2023)")

    def test_numbers_and_symbols_are_untouched(self):
        self.m("28 years later", "28 Years Later")
        self.m("apocalipsis z", "Apocalipsis Z")


class MoveRequestEncoding(unittest.TestCase):
    """How the move is put on the wire.

    DSM's FileStation API takes list parameters as a JSON array and paths as
    JSON strings. Sending a bare comma-joined blob made it read the whole thing
    as one literal filename, and every move failed with error 408, "no such file
    or directory" — after 663 renames had already succeeded.
    """

    def calls_for(self, paths, dest):
        import library_tidy as lt
        seen = []

        class FakeDSM(lt.DSM):
            def __init__(self):
                self.base, self.sid, self.ctx = "https://nas", "sid", None

            def _call(self, cgi, params):
                seen.append(params)
                if params.get("method") == "start":
                    return {"taskid": "t1"}
                return {"finished": True, "errors": []}

        FakeDSM().move(paths, dest)
        return seen

    def test_path_is_a_json_array_and_dest_a_json_string(self):
        calls = self.calls_for(["/movie/a.mkv", "/movie/a.srt"], "/movie/A (2024)")
        start = calls[0]
        self.assertEqual(start["path"], '["/movie/a.mkv","/movie/a.srt"]')
        self.assertEqual(start["dest_folder_path"], '"/movie/A (2024)"')

    def test_a_single_file_is_still_an_array(self):
        start = self.calls_for(["/movie/a.mkv"], "/movie/A (2024)")[0]
        self.assertEqual(start["path"], '["/movie/a.mkv"]')

    def test_names_with_quotes_or_backslashes_are_escaped(self):
        start = self.calls_for(['/movie/we"ird.mkv'], "/movie/X")[0]
        self.assertEqual(start["path"], '["/movie/we\\"ird.mkv"]')

    def test_the_move_is_polled_to_completion(self):
        calls = self.calls_for(["/movie/a.mkv"], "/movie/X")
        self.assertEqual(calls[0]["method"], "start")
        self.assertEqual(calls[1]["method"], "status")
        self.assertEqual(calls[1]["taskid"], "t1")

    def test_it_never_overwrites(self):
        start = self.calls_for(["/movie/a.mkv"], "/movie/X")[0]
        self.assertEqual(start["overwrite"], "false")
        self.assertEqual(start["remove_src"], "true")

    def test_a_dsm_error_code_is_reported_in_words(self):
        import library_tidy as lt

        class FailingDSM(lt.DSM):
            def __init__(self):
                self.base, self.sid, self.ctx = "https://nas", "sid", None

            def _call(self, cgi, params):
                if params.get("method") == "start":
                    return {"taskid": "t1"}
                return {"finished": True, "errors": [{"code": 414, "path": "/movie/a.mkv"}]}

        with self.assertRaises(RuntimeError) as cm:
            FailingDSM().move(["/movie/a.mkv"], "/movie/X")
        # 414 is "already exists" — the message must say so, not just the number.
        self.assertIn("already exists", str(cm.exception).lower())
