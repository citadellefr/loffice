// Validates the packages L'Office wrote against the Open XML SDK, the reference
// for what Office opens without offering a repair. An original file may
// already break the schema; a rewrite fails only on errors it adds.
//
//   dotnet run -- <originals> <rewritten>

using DocumentFormat.OpenXml;
using DocumentFormat.OpenXml.Packaging;
using DocumentFormat.OpenXml.Validation;

if (args.Length != 2)
{
    Console.Error.WriteLine("usage: validate <originals> <rewritten>");
    return 2;
}

var validator = new OpenXmlValidator(FileFormatVersions.Microsoft365);
// Documents L'Office edited may lose parts, a deleted slide; the parts it added
// must be valid.
var edited = Environment.GetEnvironmentVariable("LOFFICE_EDITED") == "1";
int files = 0, skipped = 0, failed = 0;
foreach (var rewritten in Directory.EnumerateFiles(args[1], "*", SearchOption.AllDirectories).Order())
{
    var name = Path.GetRelativePath(args[1], rewritten);
    var before = Check(Path.Combine(args[0], name));
    if (before == null)
    {
        skipped++;
        continue;
    }
    files++;
    var after = Check(rewritten);
    // parts the SDK could not see in the original, under a name L'Office
    // normalized, bring their own errors
    var hidden = Hidden(Path.Combine(args[0], name));
    var added = after == null
        ? ["the rewritten package does not open"]
        // the calculation chain goes when cells are rewritten: Excel makes
        // it again
        : before.Parts.Except(after.Parts).Where(p => !edited && !p.EndsWith("/calcChain.xml")).Select(p => $"{p} lost")
            .Concat(Added(before.Errors, after.Errors.Where(e => before.Parts.Contains(e.Part)), edited))
            .Concat(after.Errors.Where(e => edited && !before.Parts.Contains(e.Part) && !hidden.Contains(e.Part)).Select(e => e.Text))
            .ToList();
    if (added.Count > 0)
    {
        failed++;
        Console.WriteLine($"{name}: {added.Count} new errors");
        foreach (var e in added.Take(10))
            Console.WriteLine($"  {e}");
    }
}
Console.WriteLine($"{files} validated, {failed} with new errors, {skipped} skipped (the original does not open)");
return failed > 0 ? 1 : 0;

// Check lists the parts of a package and its validation errors, null when
// the SDK cannot open it at all.
Report? Check(string path)
{
    OpenXmlPackage? doc;
    try
    {
        doc = Open(path);
    }
    catch (Exception)
    {
        return null;
    }
    if (doc == null)
        return null;
    using (doc)
    {
        HashSet<string> parts = [""];
        HashSet<Error> errors;
        try
        {
            parts.UnionWith(doc.GetAllParts().Select(p => p.Uri.ToString()));
            errors = validator.Validate(doc)
                .Select(e => new Error(e.Part?.Uri.ToString() ?? "", $"{e.Part?.Uri} {e.Path?.XPath} {e.Id}: {e.Description}"))
                .ToHashSet();
        }
        catch (Exception e)
        {
            errors = [new Error("", $"validation stopped: {e.GetType().Name}: {e.Message}")];
        }
        return new Report(parts, errors);
    }
}

// Added are the errors after that were not before. Runs of the same
// formatting merged move the elements after them, so an error counts as
// the same wherever it moved in its part: errors are compared by their
// text without positions, paths or lines, as many after as before. An edit
// copies what it splits, errors and all: in an edited document an error
// that was there before may come back more often, or inside a revision
// the edit tracks.
static IEnumerable<string> Added(HashSet<Error> before, IEnumerable<Error> after, bool edited)
{
    var count = before.GroupBy(e => Unplaced(e.Text)).ToDictionary(g => g.Key, g => edited ? int.MaxValue : g.Count());
    foreach (var e in after.OrderBy(e => e.Text))
    {
        var key = Unplaced(e.Text);
        if (count.TryGetValue(key, out var n) && n > 0)
            count[key] = n - 1;
        else
            yield return e.Text;
    }
}

static string Unplaced(string text) => System.Text.RegularExpressions.Regex.Replace(text, @"\[\d+\]|Line \d+, position \d+|/w:(ins|del)(?=\[)", "");

// Hidden are the parts of a package stored under a name with backslashes,
// as they read once normalized.
static HashSet<string> Hidden(string path)
{
    try
    {
        using var zip = System.IO.Compression.ZipFile.OpenRead(path);
        return zip.Entries.Where(e => e.FullName.Contains('\\')).Select(e => "/" + e.FullName.Replace('\\', '/')).ToHashSet(StringComparer.OrdinalIgnoreCase);
    }
    catch (Exception)
    {
        return [];
    }
}

static OpenXmlPackage? Open(string path) => Path.GetExtension(path).ToLowerInvariant() switch
{
    ".docx" or ".docm" or ".dotx" or ".dotm" => WordprocessingDocument.Open(path, false),
    ".xlsx" or ".xlsm" or ".xltx" or ".xltm" => SpreadsheetDocument.Open(path, false),
    ".pptx" or ".pptm" or ".potx" or ".potm" or ".ppsx" or ".ppsm" => PresentationDocument.Open(path, false),
    _ => null,
};

record Error(string Part, string Text);

// Parts holds "" for the package itself.
record Report(HashSet<string> Parts, HashSet<Error> Errors);
