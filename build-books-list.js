const fs = require('fs');
const path = require('path');
const AdmZip = require('adm-zip');
const xml2js = require('xml2js');

// Chemin vers le dossier ressources
const ressourcesPath = path.join(__dirname, 'ressources');
const outputPath = path.join(__dirname, 'books.json');

console.log('🔍 Recherche des fichiers EPUB...');

// Fonction pour extraire les métadonnées d'un fichier EPUB
async function extractEpubMetadata(filePath) {
  try {
    const zip = new AdmZip(filePath);
    const zipEntries = zip.getEntries();
    
    // Trouver le fichier container.xml pour localiser le content.opf
    let containerEntry = zipEntries.find(e => e.entryName === 'META-INF/container.xml');
    if (!containerEntry) {
      console.warn(`⚠️  Pas de container.xml dans ${path.basename(filePath)}`);
      return null;
    }
    
    // Parser container.xml pour trouver le chemin du content.opf
    const containerXml = containerEntry.getData().toString('utf8');
    const containerData = await xml2js.parseStringPromise(containerXml);
    const opfPath = containerData?.container?.rootfiles?.[0]?.rootfile?.[0]?.$?.['full-path'];
    
    if (!opfPath) {
      console.warn(`⚠️  Impossible de trouver le chemin OPF dans ${path.basename(filePath)}`);
      return null;
    }
    
    // Lire le fichier content.opf
    const opfEntry = zipEntries.find(e => e.entryName === opfPath);
    if (!opfEntry) {
      console.warn(`⚠️  Fichier OPF introuvable: ${opfPath}`);
      return null;
    }
    
    const opfXml = opfEntry.getData().toString('utf8');
    const opfData = await xml2js.parseStringPromise(opfXml);
    
    // Extraire les métadonnées
    const metadata = opfData?.package?.metadata?.[0];
    if (!metadata) {
      return null;
    }
    
    // Fonction helper pour extraire le texte d'un champ
    const getText = (field) => {
      if (!field || !field[0]) return null;
      return typeof field[0] === 'string' ? field[0] : field[0]._ || null;
    };
    
    // Extraire les créateurs (auteurs)
    let authors = [];
    if (metadata['dc:creator']) {
      authors = metadata['dc:creator'].map(creator => getText([creator])).filter(Boolean);
    }
    
    // Extraire les sujets/tags
    let subjects = [];
    if (metadata['dc:subject']) {
      subjects = metadata['dc:subject'].map(subject => getText([subject])).filter(Boolean);
    }
    
    return {
      title: getText(metadata['dc:title']) || null,
      authors: authors.length > 0 ? authors : null,
      language: getText(metadata['dc:language']) || null,
      publisher: getText(metadata['dc:publisher']) || null,
      description: getText(metadata['dc:description']) || null,
      subjects: subjects.length > 0 ? subjects : null,
      date: getText(metadata['dc:date']) || null,
      identifier: getText(metadata['dc:identifier']) || null,
      rights: getText(metadata['dc:rights']) || null
    };
    
  } catch (error) {
    console.error(`❌ Erreur lors de l'extraction des métadonnées de ${path.basename(filePath)}:`, error.message);
    return null;
  }
}

// Fonction principale
async function buildBooksList() {
  try {
    // Vérifier si le dossier existe
    if (!fs.existsSync(ressourcesPath)) {
      console.error('❌ Le dossier "ressources" n\'existe pas !');
      fs.writeFileSync(outputPath, JSON.stringify({ files: [], count: 0 }, null, 2));
      process.exit(1);
    }

    // Lire le contenu du dossier
    const files = fs.readdirSync(ressourcesPath);

    // Filtrer les fichiers EPUB
    const epubFilenames = files.filter(file => {
      const filePath = path.join(ressourcesPath, file);
      const isFile = fs.statSync(filePath).isFile();
      const isEpub = path.extname(file).toLowerCase() === '.epub';
      return isFile && isEpub;
    });

    console.log(`📚 ${epubFilenames.length} fichier(s) EPUB trouvé(s)`);
    
    // Traiter chaque fichier EPUB
    const epubFiles = [];
    for (const file of epubFilenames) {
      const filePath = path.join(ressourcesPath, file);
      const stats = fs.statSync(filePath);
      
      console.log(`   🔄 Traitement: ${file}...`);
      
      // Extraire les métadonnées
      const metadata = await extractEpubMetadata(filePath);
      
      epubFiles.push({
        name: file,
        displayName: metadata?.title || path.basename(file, '.epub'),
        path: `./ressources/${file}`,
        size: stats.size,
        modifiedDate: stats.mtime.toISOString(),
        metadata: metadata
      });
      
      if (metadata?.title) {
        console.log(`      ✓ Titre: ${metadata.title}`);
        if (metadata.authors) {
          console.log(`      ✓ Auteur(s): ${metadata.authors.join(', ')}`);
        }
      } else {
        console.log(`      ⚠️  Métadonnées non disponibles`);
      }
    }

    // Trier par titre (ou nom de fichier si pas de titre)
    epubFiles.sort((a, b) => a.displayName.localeCompare(b.displayName));

    // Créer l'objet de sortie
    const output = {
      generated: new Date().toISOString(),
      count: epubFiles.length,
      files: epubFiles
    };

    // Écrire le fichier JSON
    fs.writeFileSync(outputPath, JSON.stringify(output, null, 2));

    console.log(`\n✅ Fichier books.json créé avec succès !`);
    console.log(`📊 ${epubFiles.length} livre(s) traité(s)`);
    
  } catch (error) {
    console.error('❌ Erreur :', error.message);
    process.exit(1);
  }
}

// Exécuter le script
buildBooksList();