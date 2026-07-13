# Checklist de livraison Linux

## Configuration GitHub du dépôt

Ces réglages administrateur ne sont pas stockés dans Git et doivent être
appliqués avant le premier tag :

- protéger la branche `main` et imposer une pull request avant fusion ;
- rendre obligatoire le contrôle `Verify and build` du workflow `CI` ;
- imposer que la branche soit à jour avant la fusion ;
- activer l'immutabilité des releases si l'option est disponible pour le dépôt.

## Validation manuelle avant `v0.1.0`

Tester le paquet Arch sur une machine Omarchy/Hyprland et sur une version
actuelle de Manjaro :

- installer le paquet avec `pacman -U` ;
- confirmer la présence de NoteVault dans le lanceur et la bonne icône ;
- confirmer le regroupement de la fenêtre sous l'identité `notevault` ;
- tester le focus et le redimensionnement de la fenêtre ;
- créer ou ouvrir un coffre et modifier une note ;
- désinstaller le paquet et confirmer que le coffre et les données utilisateur
  sont toujours présents.

## Publication

1. Lancer manuellement `Linux packages` avec la version `0.1.0` et inspecter
   les artefacts conservés.
2. Effectuer la validation manuelle ci-dessus avec le paquet produit.
3. Créer le tag annoté `v0.1.0` depuis le commit validé et le pousser.
4. Attendre le succès de toute la matrice de paquets.
5. Vérifier la release, les deux paquets et `SHA256SUMS` sur GitHub.
