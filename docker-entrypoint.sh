#!/bin/sh
set -e

echo "📚 Démarrage du service library..."

# Vérifier si le dossier ressources est monté
if [ -d "/app/ressources" ] && [ "$(ls -A /app/ressources/*.epub 2>/dev/null | wc -l)" -gt 0 ]; then
  echo "✅ Dossier ressources détecté, génération de books.json..."
  cd /app && node build-books-list.js
  cp /app/books.json /usr/share/nginx/html/books.json
  echo "✅ books.json généré et copié."
else
  echo "⚠️  Aucun EPUB trouvé dans /app/ressources — catalogue vide."
fi

# Créer un lien symbolique pour servir les EPUBs directement
if [ -d "/app/ressources" ]; then
  ln -sf /app/ressources /usr/share/nginx/html/ressources
  echo "✅ Dossier ressources exposé via Nginx."
fi

echo "🚀 Lancement de Nginx..."
exec nginx -g "daemon off;"
